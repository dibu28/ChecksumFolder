# CheckSumFolder

CheckSumFolder is a command line tool written in Go that scans a directory
recursively, computes the SHA1 checksum of every file and writes the results to
an output text file. The tool can resume interrupted runs and also verify files
against a previously generated list of hashes.

## Usage

### Generate checksums
```
CheckSumFolder -dir /path/to/dir -out hashes.txt
```
If `hashes.txt` already contains results, existing entries are skipped so the
operation can be resumed.

Use `-progress` to periodically print how many files have been processed.

Example:
```
CheckSumFolder -dir /path/to/dir -out hashes.txt -progress
```

### Verify checksums
```
CheckSumFolder -verify -dir /path/to/dir -list hashes.txt [-verbose]
```
The `-dir` flag specifies the folder containing the files to verify. Each line
in `hashes.txt` may contain absolute paths from a different system. During
verification the tool ignores directories and path separators, using only the
file name from the list when building the full path under `-dir`.
Use `-verbose` to print the status of every file. Without it, only mismatches
are printed or a message that everything matches. Add `-progress` to show
verification progress. Verification runs in parallel across all CPU cores to
speed up processing on large directory trees.

Example:
```
CheckSumFolder -verify -dir /path/to/dir -list hashes.txt -progress
```
