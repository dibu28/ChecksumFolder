//go:build !cgo || (!amd64 && !arm64 && !arm)

package wyhashc

import "github.com/zeebo/wyhash"

func Sum64(b []byte) uint64 {
	return wyhash.Hash(b, 0)
}
