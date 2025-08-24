//go:build cgo && (amd64 || arm64)

package blake3c

/*
#cgo CFLAGS: -O3 -std=c99 -fno-stack-protector -fPIC
#cgo amd64 CFLAGS: -DBLAKE3_NO_SSE2 -DBLAKE3_NO_SSE41 -DBLAKE3_NO_AVX2 -DBLAKE3_NO_AVX512
#cgo arm64 CFLAGS: -DBLAKE3_USE_NEON
#include "blake3.h"
#include "blake3_impl.h"
#include "lib/blake3.c"
#include "lib/blake3_dispatch.c"
#include "lib/blake3_portable.c"
#if BLAKE3_USE_NEON
#include "lib/blake3_neon.c"
#endif
*/
import "C"
import "unsafe"

type Hasher struct{ h C.blake3_hasher }

// BLAKE3Init initializes a new hashing state.
func BLAKE3Init() *Hasher {
	h := new(Hasher)
	C.blake3_hasher_init(&h.h)
	return h
}

// BLAKE3Update adds more data to the hash state.
func BLAKE3Update(h *Hasher, b []byte) {
	if len(b) == 0 {
		return
	}
	C.blake3_hasher_update(&h.h, unsafe.Pointer(&b[0]), C.size_t(len(b)))
}

// BLAKE3Finalize finishes the hash and returns a 32-byte digest.
func BLAKE3Finalize(h *Hasher) [32]byte {
	var out [32]byte
	C.blake3_hasher_finalize(&h.h, (*C.uint8_t)(unsafe.Pointer(&out[0])), C.size_t(len(out)))
	return out
}

func (h *Hasher) Reset() { C.blake3_hasher_reset(&h.h) }
func (h *Hasher) Write(p []byte) (int, error) {
	BLAKE3Update(h, p)
	return len(p), nil
}

func (h *Hasher) Sum(b []byte) []byte {
	sum := BLAKE3Finalize(h)
	return append(b, sum[:]...)
}

func (h *Hasher) Size() int      { return 32 }
func (h *Hasher) BlockSize() int { return 64 }
