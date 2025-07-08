# CheckSumFolder

CheckSumFolder is a command line tool written in Go that scans a directory
recursively, computes checksums of every file and writes the results to an
output text file. The tool can resume interrupted runs and also verify files
against a previously generated list of hashes. By default SHA1 is used but the
hash algorithm can be changed.

## Usage

### Generate checksums
```
CheckSumFolder -dir /path/to/dir [-list hashes.txt] [-hash sha256]
```
If `-list` is omitted the results are printed to the console. When a file is
specified and it already contains results, existing entries are skipped so the
operation can be resumed. Use `-hash` to select the hashing algorithm. Allowed
values are `sha1`, `sha256`, `blake3`, `xxhash`, `highway64`, `highway128` and `highway256`.
When using a HighwayHash variant you can provide a custom key via the `-hkey`
flag. The key must be 32 bytes encoded as hex or base64. If omitted the
default key `AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=` (base64) is used.
HighwayHash assembly accelerates only x86 and ARM64 platforms.


## TODO

- add option to save to jsol format
Use `-progress` to periodically print how many files have been processed.
Use `-json` to write results in JSONL format where each line is a JSON object
containing `hash` and `path` fields.

Example writing to a file:
```
CheckSumFolder -dir /path/to/dir -list hashes.txt -progress
```

### Verify checksums
```
CheckSumFolder -verify -dir /path/to/dir -list hashes.txt [-verbose]
```
The `-dir` flag specifies the folder containing the files to verify. Each line

in `hashes.txt` may contain absolute paths from a different system. During
verification the program removes any common directory prefix from the paths in
the list and joins the remainder with the directory provided via `-dir`. This
allows verifying files across machines even when the root folders differ.

Use `-verbose` to print the status of every file. Without it, only mismatches
are printed or a message that everything matches. Add `-progress` to show
verification progress. Verification runs in parallel across all CPU cores to
speed up processing on large directory trees.
Use `-json` when verifying to read the checksum list in JSONL format.
When verifying with a HighwayHash algorithm pass the same key using `-hkey`. If
the flag is omitted the same default key is assumed.

Example:
```
CheckSumFolder -verify -dir /path/to/dir -list hashes.txt -progress
```

### CPU Optimizations

ChecksumFolder detects available CPU features using the
[`cpuid`](https://github.com/klauspost/cpuid) library. When SIMD
instructions like SSE2 on x86 or ASIMD/NEON on ARM64 are present the
program uses the accelerated `sha256-simd` implementation.
HighwayHash also ships optimized assembly for x86 and ARM64 CPUs. No official
armv7 assembly is provided.
On older CPUs without these capabilities it transparently falls back to Go's
standard implementations. This happens automatically at startup and
works across different architectures.
