# CheckSumFolder

CheckSumFolder is a command line tool written in Go that scans a directory
recursively, computes the SHA1 checksum of every file and writes the results to
an output text file. The tool can resume interrupted runs and also verify files
against a previously generated list of hashes.

## Usage

### Generate checksums
```
CheckSumFolder -dir /path/to/dir [-list hashes.txt]
```
If `-list` is omitted the results are printed to the console. When a file is
specified and it already contains results, existing entries are skipped so the
operation can be resumed.


## TODO

- add option to save to jsol format
- add option to change hash type crc32/md5/sha1/sha256/blake3/etc
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

Example:
```
CheckSumFolder -verify -dir /path/to/dir -list hashes.txt -progress
```

## TODO

- add option to change hash type crc32/md5/sha1/sha256/blake3/etc
