package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
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
	list := flag.String("list", "", "checksum list file")
	verify := flag.Bool("verify", false, "verify mode")
	verbose := flag.Bool("verbose", false, "verbose verify output")
	progress := flag.Bool("progress", false, "show progress updates")
	jsonl := flag.Bool("json", false, "output in JSONL format")
	flag.Parse()

	if *verify {
		if *list == "" {
			log.Fatal("-list required in verify mode")
		}
		if err := verifyChecksums(*dir, *list, *verbose, *progress, *jsonl); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := generateChecksums(*dir, *list, *progress, *jsonl); err != nil {
			log.Fatal(err)
		}
	}
}

func generateChecksums(dir, output string, progress, jsonOut bool) error {
	processed := map[string]bool{}
	toFile := output != ""
	var file *os.File
	var err error

	if toFile {
		if f, err := os.Open(output); err == nil {
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if jsonOut {
					var e struct {
						Hash string `json:"hash"`
						Path string `json:"path"`
					}
					if err := json.Unmarshal([]byte(line), &e); err == nil {
						processed[e.Path] = true
					}
				} else {
					parts := strings.SplitN(line, "\t", 2)
					if len(parts) == 2 {
						processed[parts[1]] = true
					}
				}
			}
			f.Close()
		}

		file, err = os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		defer file.Close()
	} else {
		file = os.Stdout
	}
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
					var line string
					if jsonOut {
						b, _ := json.Marshal(map[string]string{"hash": hash, "path": path})
						line = string(b) + "\n"
					} else {
						line = fmt.Sprintf("%s\t%s\n", hash, path)
					}
					mu.Lock()
					if _, err := file.WriteString(line); err == nil && toFile {
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

func verifyChecksums(dir, listfile string, verbose, progress, jsonIn bool) error {
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
		if jsonIn {
			var e struct {
				Hash string `json:"hash"`
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(line), &e); err == nil {
				p := strings.ReplaceAll(e.Path, "\\", "/")
				entries = append(entries, entry{hash: e.Hash, path: p})
			}
		} else {
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) == 2 {
				// Normalize all backslashes to forward slashes.
				// This is crucial for consistent parsing of paths from Windows.
				p := strings.ReplaceAll(parts[1], "\\", "/")
				entries = append(entries, entry{hash: parts[0], path: p})
			}
		}
	}
	f.Close()

	expected := map[string]string{}
	var pathsToProcess []string

	// Get the absolute path of the -dir argument
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for -dir: %w", err)
	}
	// Clean the absolute directory path for consistent comparison
	absDir = filepath.Clean(absDir)

	for _, e := range entries {
		var actualPath string
		// Check if the path from the list file starts with a Windows drive letter (e.g., "H:/")
		isWindowsDrivePath := len(e.path) >= 2 && e.path[1] == ':' && (e.path[0] >= 'A' && e.path[0] <= 'Z' || e.path[0] >= 'a' && e.path[0] <= 'z')

		if isWindowsDrivePath {
			// If it's a Windows drive path, strip the drive letter and leading slash/backslash
			// and treat the rest as relative to the -dir.
			tempPath := e.path[2:]
			if strings.HasPrefix(tempPath, "/") || strings.HasPrefix(tempPath, "\\") {
				tempPath = tempPath[1:]
			}
			// Now tempPath is like "_YD_Photo/Фото и видео/..."
			// Apply the dirBase trimming logic to tempPath
			dirBase := filepath.Base(absDir)
			if dirBase != "" && strings.HasPrefix(tempPath, dirBase+"/") {
				actualPath = filepath.Join(absDir, strings.TrimPrefix(tempPath, dirBase+"/"))
			} else {
				actualPath = filepath.Join(absDir, tempPath)
			}
		} else if filepath.IsAbs(e.path) {
			// If it's a Unix-style absolute path (starts with /), use it directly.
			// This assumes the absolute path is valid on the current system.
			actualPath = e.path
		} else {
			// It's a relative path (e.g., "_YD_Photo/..." or "subfolder/...")
			// Join it with the absolute -dir.
			// We still need the logic to trim dirBase if present in e.path.
			dirBase := filepath.Base(absDir)
			if dirBase != "" && strings.HasPrefix(e.path, dirBase+"/") {
				actualPath = filepath.Join(absDir, strings.TrimPrefix(e.path, dirBase+"/"))
			} else {
				actualPath = filepath.Join(absDir, e.path)
			}
		}
		expected[actualPath] = e.hash
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
					fmt.Printf("ERROR: %s: %v\n", path, hErr) // Add this line
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
