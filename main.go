package main

import (
	"bufio"
	"crypto/sha1"
	stdsha256 "crypto/sha256"
	"encoding/base64"
	"encoding/binary"
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

	"CheckSumFolder/rapidhashc"
	"CheckSumFolder/wyhashc"
	"github.com/cespare/xxhash/v2"
	blake2b "github.com/minio/blake2b-simd"
	"github.com/minio/highwayhash"
	sha256 "github.com/minio/sha256-simd"
	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"

	"CheckSumFolder/blake3c"
	"CheckSumFolder/t1ha"

	"hash"
)

var highwayKey = []byte{
	0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
	0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
	0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
	0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
}

const defaultHighwayKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func main() {
	dir := flag.String("dir", ".", "directory to scan")
	list := flag.String("list", "", "checksum list file")
	verify := flag.Bool("verify", false, "verify mode")
	verbose := flag.Bool("verbose", false, "verbose verify output")
	progress := flag.Bool("progress", false, "show progress updates")
	jsonl := flag.Bool("json", false, "output in JSONL format")
	hkeyFlag := flag.String("hkey", defaultHighwayKey, "hex or base64 HighwayHash key")
	algo := flag.String("hash", "sha1", "hash algorithm: sha1|sha256|blake2b|blake3|xxhash|xxh3|xxh128|t1ha1|t1ha2|highway64|highway128|highway256|wyhash|rapidhash")
	flag.Parse()

	if k, err := hex.DecodeString(*hkeyFlag); err == nil {
		if len(k) != 32 {
			log.Fatal("highwayhash key must be 32 bytes")
		}
		highwayKey = k
	} else if k, err := base64.StdEncoding.DecodeString(*hkeyFlag); err == nil {
		if len(k) != 32 {
			log.Fatal("highwayhash key must be 32 bytes")
		}
		highwayKey = k
	} else {
		log.Fatal("invalid hkey encoding")
	}

	if *verify {
		if *list == "" {
			log.Fatal("-list required in verify mode")
		}
		if err := verifyChecksums(*dir, *list, *verbose, *progress, *jsonl, *algo); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := generateChecksums(*dir, *list, *progress, *jsonl, *algo); err != nil {
			log.Fatal(err)
		}
	}
}

func generateChecksums(dir, output string, progress, jsonOut bool, algo string) error {
	processed := map[string]bool{}
	toFile := output != ""
	var file *os.File
	var writer *bufio.Writer
	const flushInterval = 100
	var lineCount int
	mu := sync.Mutex{}
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
		writer = bufio.NewWriterSize(file, 64*1024)
		defer func() {
			mu.Lock()
			writer.Flush()
			file.Sync()
			file.Close()
			mu.Unlock()
		}()
	} else {
		file = os.Stdout
		writer = bufio.NewWriter(file)
		defer func() {
			mu.Lock()
			writer.Flush()
			mu.Unlock()
		}()
	}
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
				hash, err := hashFile(path, algo)
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
					if _, err := writer.WriteString(line); err == nil {
						lineCount++
						if lineCount%flushInterval == 0 {
							writer.Flush()
							if toFile {
								file.Sync()
							}
						}
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
	mu.Lock()
	writer.Flush()
	if toFile {
		file.Sync()
	}
	mu.Unlock()
	if ticker != nil {
		ticker.Stop()
		fmt.Printf("%d/%d\n", processedCount, total)
	}
	return nil
}

func verifyChecksums(dir, listfile string, verbose, progress, jsonIn bool, algo string) error {
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
				hash, hErr := hashFile(path, algo)
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

func hashFile(path, algo string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	alg := strings.ToLower(algo)

	if alg == "xxh128" {
		h := xxh3.New()
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
		sum := h.Sum128().Bytes()
		return hex.EncodeToString(sum[:]), nil
	} else if alg == "t1ha2" {
		b, err := io.ReadAll(f)
		if err != nil {
			return "", err
		}
		lo, hi := t1ha.Sum128(b, 0)
		sum := make([]byte, 16)
		binary.BigEndian.PutUint64(sum[:8], hi)
		binary.BigEndian.PutUint64(sum[8:], lo)
		return hex.EncodeToString(sum), nil
	}

	var h hash.Hash
	switch alg {
	case "sha1":
		h = sha1.New()
	case "sha256":
		if useStdSHA256 {
			h = stdsha256.New()
		} else {
			h = sha256.New()
		}
	case "blake2b":
		h = blake2b.New512()
	case "blake3":
		if useBlake3C {
			ch := blake3c.BLAKE3Init()
			if _, err := io.Copy(ch, f); err != nil {
				return "", err
			}
			return hex.EncodeToString(ch.Sum(nil)), nil
		}
		h = blake3.New()
	case "xxhash":
		h = xxhash.New()
	case "xxh3":
		h = xxh3.New()
	case "t1ha1":
		// t1ha1 processes a byte slice entirely in memory
		b, err := io.ReadAll(f)
		if err != nil {
			return "", err
		}
		sum := t1ha.Sum64(b, 0)
		return fmt.Sprintf("%016x", sum), nil
	case "wyhash":
		b, err := io.ReadAll(f)
		if err != nil {
			return "", err
		}
		sum := wyhashc.Sum64(b)
		return fmt.Sprintf("%016x", sum), nil
	case "rapidhash":
		b, err := io.ReadAll(f)
		if err != nil {
			return "", err
		}
		sum := rapidhashc.Sum64(b)
		return fmt.Sprintf("%016x", sum), nil
	case "highway64":
		hw, err := highwayhash.New64(highwayKey)
		if err != nil {
			return "", err
		}
		h = hw
	case "highway128":
		hw, err := highwayhash.New128(highwayKey)
		if err != nil {
			return "", err
		}
		h = hw
	case "highway256":
		hw, err := highwayhash.New(highwayKey)
		if err != nil {
			return "", err
		}
		h = hw
	default:
		return "", fmt.Errorf("unknown hash algorithm: %s", algo)
	}
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
