package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	dir := flag.String("dir", ".", "directory to scan")
	out := flag.String("out", "hashes.txt", "output file (for hashing)")
	verify := flag.Bool("verify", false, "verify mode")
	list := flag.String("list", "", "list file for verify mode")
	verbose := flag.Bool("verbose", false, "verbose verify output")
	progress := flag.Bool("progress", false, "show progress updates")
	flag.Parse()

	if *verify {
		if *list == "" {
			log.Fatal("-list required in verify mode")
		}
		if err := verifyChecksums(*dir, *list, *verbose, *progress); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := generateChecksums(*dir, *out, *progress); err != nil {
			log.Fatal(err)
		}
	}
}

func generateChecksums(dir, output string, progress bool) error {
	processed := map[string]bool{}
	if f, err := os.Open(output); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) == 2 {
				processed[parts[1]] = true
			}
		}
		f.Close()
	}

	file, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	mu := sync.Mutex{}

	var paths []string
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && !processed[path] {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	total := len(paths)
	var processedCount int64

	jobs := make(chan string)
	wg := sync.WaitGroup{}
	workers := runtime.NumCPU()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				hash, err := hashFile(path)
				if err != nil {
					log.Printf("%v", err)
				} else {
					line := fmt.Sprintf("%s\t%s\n", hash, path)
					mu.Lock()
					if _, err := file.WriteString(line); err == nil {
						file.Sync()
					}
					mu.Unlock()
				}
				atomic.AddInt64(&processedCount, 1)
			}
		}()
	}

	var ticker *time.Ticker
	if progress && total > 0 {
		ticker = time.NewTicker(time.Second)
		go func() {
			for range ticker.C {
				fmt.Printf("%d/%d\n", atomic.LoadInt64(&processedCount), total)
			}
		}()
	}

	for _, p := range paths {
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	if ticker != nil {
		ticker.Stop()
		fmt.Printf("%d/%d\n", processedCount, total)
	}
	return nil
}

func verifyChecksums(dir, listfile string, verbose, progress bool) error {
	type entry struct {
		hash string
		path string
	}
	var entries []entry

	f, err := os.Open(listfile)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(f)
	var prefix string
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			p := strings.ReplaceAll(parts[1], "\\", "/")
			if first {
				prefix = p
				first = false
			} else {
				prefix = commonPrefix(prefix, p)
			}
			entries = append(entries, entry{hash: parts[0], path: p})
		}
	}
	f.Close()

	if i := strings.LastIndex(prefix, "/"); i >= 0 {
		prefix = prefix[:i+1]
	} else {
		prefix = ""
	}

	expected := map[string]string{}
	var paths []string
	for _, e := range entries {
		rel := strings.TrimPrefix(e.path, prefix)
		expected[rel] = e.hash
		paths = append(paths, rel)
	}

	var match, mismatch int

	total := len(paths)
	var processedCount int64

	type result struct {
		path   string
		status string
		ok     bool
	}

	jobs := make(chan string)
	workers := runtime.NumCPU()
	results := make(chan result, workers)
	done := make(chan struct{})
	wg := sync.WaitGroup{}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range jobs {
				path := filepath.Join(dir, name)
				exp, ok := expected[name]
				hash, hErr := hashFile(path)
				r := result{path: path}
				if hErr != nil {
					r.status = hErr.Error()
				} else if !ok || exp != hash {
					r.status = "MISMATCH"
				} else {
					r.status = "OK"
					r.ok = true
				}
				results <- r
				atomic.AddInt64(&processedCount, 1)
			}
		}()
	}

	go func() {
		for r := range results {
			if r.ok {
				match++
			} else {
				mismatch++
			}
			if verbose || r.status == "MISMATCH" || (r.status == "OK" && !r.ok) {
				fmt.Printf("%s %s\n", r.path, r.status)
			}
		}
		done <- struct{}{}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	var ticker *time.Ticker
	if progress && total > 0 {
		ticker = time.NewTicker(time.Second)
		go func() {
			for range ticker.C {
				fmt.Printf("%d/%d\n", atomic.LoadInt64(&processedCount), total)
			}
		}()
	}

	for _, p := range paths {
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	if ticker != nil {
		ticker.Stop()
		fmt.Printf("%d/%d\n", processedCount, total)
	}

	<-done

	if !verbose {
		if mismatch == 0 {
			fmt.Println("All files match")
		}
	}
	fmt.Printf("Total:%d Match:%d Mismatch:%d\n", total, match, mismatch)
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func commonPrefix(a, b string) string {
	max := len(a)
	if len(b) < max {
		max = len(b)
	}
	i := 0
	for i < max && a[i] == b[i] {
		i++
	}
	return a[:i]
}
