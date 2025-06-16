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
)

func main() {
	dir := flag.String("dir", ".", "directory to scan")
	out := flag.String("out", "hashes.txt", "output file (for hashing)")
	verify := flag.Bool("verify", false, "verify mode")
	list := flag.String("list", "", "list file for verify mode")
	verbose := flag.Bool("verbose", false, "verbose verify output")
	flag.Parse()

	if *verify {
		if *list == "" {
			log.Fatal("-list required in verify mode")
		}
		if err := verifyChecksums(*dir, *list, *verbose); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := generateChecksums(*dir, *out); err != nil {
			log.Fatal(err)
		}
	}
}

func generateChecksums(dir, output string) error {
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
					continue
				}
				line := fmt.Sprintf("%s\t%s\n", hash, path)
				mu.Lock()
				if _, err := file.WriteString(line); err == nil {
					file.Sync()
				}
				mu.Unlock()
			}
		}()
	}

	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if !processed[path] {
				jobs <- path
			}
		}
		return nil
	})
	if err != nil {
		close(jobs)
		wg.Wait()
		return err
	}
	close(jobs)
	wg.Wait()
	return nil
}

func verifyChecksums(dir, listfile string, verbose bool) error {
	expected := map[string]string{}
	f, err := os.Open(listfile)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			expected[parts[1]] = parts[0]
		}
	}
	f.Close()

	var total, match, mismatch int

	type result struct {
		path   string
		status string
		ok     bool
	}

	jobs := make(chan string)
	results := make(chan result)
	wg := sync.WaitGroup{}
	workers := runtime.NumCPU()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				exp, ok := expected[path]
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
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		jobs <- path
		return nil
	})
	close(jobs)
	if err != nil {
		for range results {
		}
		return err
	}

	for r := range results {
		total++
		if r.ok {
			match++
		} else {
			mismatch++
		}
		if verbose || r.status == "MISMATCH" || r.status == "OK" && !r.ok {
			fmt.Printf("%s %s\n", r.path, r.status)
		}
	}

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
