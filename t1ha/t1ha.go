package t1ha

/*
#cgo CFLAGS: -O3 -std=c11 -fno-stack-protector -fPIC
#cgo LDFLAGS:
#include <stdint.h>
#include "t1ha.h"
#include "lib/t1ha1.c"
#include "lib/t1ha2.c"
*/
import "C"
import "unsafe"

// Sum64 computes the t1ha1 hash of data with the given seed.
func Sum64(data []byte, seed uint64) uint64 {
	var ptr unsafe.Pointer
	if len(data) > 0 {
		ptr = unsafe.Pointer(&data[0])
	}
	return uint64(C.t1ha1_le(ptr, C.size_t(len(data)), C.uint64_t(seed)))
}

// Sum64T1ha2 computes the 64-bit t1ha2 hash of data with the given seed.
func Sum64T1ha2(data []byte, seed uint64) uint64 {
	var ptr unsafe.Pointer
	if len(data) > 0 {
		ptr = unsafe.Pointer(&data[0])
	}
	return uint64(C.t1ha2_atonce(ptr, C.size_t(len(data)), C.uint64_t(seed)))
}

// Sum128 computes the 128-bit t1ha2 hash of data with the given seed.
// It returns the low and high 64-bit parts of the resulting hash.
func Sum128(data []byte, seed uint64) (low uint64, high uint64) {
	var ptr unsafe.Pointer
	if len(data) > 0 {
		ptr = unsafe.Pointer(&data[0])
	}
	var highPart C.uint64_t
	lowPart := C.t1ha2_atonce128(&highPart, ptr, C.size_t(len(data)), C.uint64_t(seed))
	return uint64(lowPart), uint64(highPart)
}
