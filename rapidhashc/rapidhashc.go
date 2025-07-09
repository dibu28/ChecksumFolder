//go:build cgo && (amd64 || arm64 || arm)

package rapidhashc

/*
#cgo CFLAGS: -O3 -std=c99 -fPIC
#cgo amd64 CFLAGS: -msse2
#cgo arm64 CFLAGS: -march=armv8-a+simd
#cgo arm CFLAGS: -mfpu=neon
#include "rapidhash.h"

static inline uint64_t rapidhash_go(const void* data, size_t len) {
    return rapidhash(data, len);
}
*/
import "C"
import "unsafe"

func Sum64(b []byte) uint64 {
	if len(b) == 0 {
		return 0
	}
	return uint64(C.rapidhash_go(unsafe.Pointer(&b[0]), C.size_t(len(b))))
}
