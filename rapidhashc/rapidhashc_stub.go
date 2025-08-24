//go:build !cgo || (!amd64 && !arm64)

package rapidhashc

import "CheckSumFolder/rapidhash"

func Sum64(b []byte) uint64 {
	return rapidhash.Hash(b)
}
