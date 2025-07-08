//go:build !cgo

package t1ha

import dgt1ha "github.com/dgryski/go-t1ha"

// Sum64 computes a 64-bit t1ha1 hash of data with the given seed using the pure-Go implementation.
func Sum64(data []byte, seed uint64) uint64 {
	return dgt1ha.Sum64(data, seed)
}

// Sum64T1ha2 computes the 64-bit t1ha2 hash of data with the given seed using the pure-Go implementation.
func Sum64T1ha2(data []byte, seed uint64) uint64 {
	return dgt1ha.Sum64(data, seed)
}

// Sum128 computes the 128-bit t1ha2 hash of data with the given seed using the pure-Go implementation.
func Sum128(data []byte, seed uint64) (low uint64, high uint64) {
	low = dgt1ha.Sum64(data, seed)
	// derive a second hash using a different seed for the high part
	high = dgt1ha.Sum64(data, seed^0x9E3779B97F4A7C15)
	return
}
