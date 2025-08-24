//go:build cgo && (amd64 || arm64)

package wyhashc

/*
#cgo CFLAGS: -O3 -std=c99
#cgo !windows CFLAGS: -fPIC
#cgo amd64 CFLAGS: -msse2
#cgo 386 CFLAGS: -msse2
#cgo arm64 CFLAGS:
#include "wyhash.h"

static inline uint64_t wyhash_go(const void* data, size_t len) {
    return wyhash(data, len, 0, _wyp);
}
*/
import "C"
import "unsafe"

func Sum64(b []byte) uint64 {
	if len(b) == 0 {
		return 0
	}
	return uint64(C.wyhash_go(unsafe.Pointer(&b[0]), C.size_t(len(b))))
}
