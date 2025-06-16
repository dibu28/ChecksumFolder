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
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			// Normalize path from list file to use OS-specific separators
			// and ensure it's clean.
			p := filepath.Clean(parts[1])
			entries = append(entries, entry{hash: parts[0], path: p})
		}
	}
	f.Close()

	expected := map[string]string{}
	var pathsToProcess []string // This will store the actual paths to hash

	for _, e := range entries {
		var actualPath string
		if filepath.IsAbs(e.path) {
			actualPath = e.path
		} else {
			// Path from list file is relative.
			// Check if it starts with the base name of the -dir flag.
			dirBase := filepath.Base(dir)
			// Ensure dirBase is not empty and e.path actually starts with it
			if dirBase != "" && strings.HasPrefix(e.path, dirBase+string(filepath.Separator)) {
				// If it does, trim the base name to avoid duplication
				actualPath = filepath.Join(dir, strings.TrimPrefix(e.path, dirBase+string(filepath.Separator)))
			} else {
				// Otherwise, just join it with dir
				actualPath = filepath.Join(dir, e.path)
			}
		}
		expected[actualPath] = e.hash // Store expected hash keyed by actual path
		pathsToProcess = append(pathsToProcess, actualPath)
	}

	var match, mismatch int
	total := len(pathsToProcess)
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
			for path := range jobs { // 'path' here is the actual path to hash
				exp, ok := expected[path] // Lookup using the actual path
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

	for _, p := range pathsToProcess { // Send actual paths to jobs channel
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
