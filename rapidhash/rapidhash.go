package rapidhash

import (
	"encoding/binary"
	"math/bits"
	"unsafe"
)

// RapidSeed is aefault seed value
const RapidSeed uint64 = 0xbdd89aa982704029

// Default secret parameters - these are the same constants from the original
var rapidSecret = [3]uint64{
	0x2d358dccaa6c78a5,
	0x8bb84b93962eacc9,
	0x4b33a62ed433d4a3,
}

// Configuration flags
const (
	// Set to true for extra protection against entropy loss (slower)
	// Set to false for maximum speed (default)
	protected = false
)

// rapidMul performs 64x64->128 bit multiplication
// When protected=false: overwrites A with low 64 bits, B with high 64 bits
// When protected=true: XORs A with low 64 bits, B with high 64 bits
func rapidMul(a, b *uint64) {
	// Use Go's built-in 64x64->128 bit multiplication
	hi, lo := bits.Mul64(*a, *b)

	if protected {
		*a ^= lo
		*b ^= hi
	} else {
		*a = lo
		*b = hi
	}
}

// rapidMix performs multiplication and returns XOR of high and low parts
func rapidMix(a, b uint64) uint64 {
	rapidMul(&a, &b)
	return a ^ b
}

// read32 reads a 32-bit value from byte slice with proper endianness
func read32(p []byte) uint64 {
	if len(p) < 4 {
		// Handle partial reads for small inputs
		var tmp [4]byte
		copy(tmp[:], p)
		return uint64(binary.LittleEndian.Uint32(tmp[:]))
	}
	return uint64(binary.LittleEndian.Uint32(p))
}

// read64 reads a 64-bit value from byte slice with proper endianness
func read64(p []byte) uint64 {
	if len(p) < 8 {
		// Handle partial reads for small inputs
		var tmp [8]byte
		copy(tmp[:], p)
		return binary.LittleEndian.Uint64(tmp[:])
	}
	return binary.LittleEndian.Uint64(p)
}

// Hash computes the rapidhash of the input data with default seed
func Hash(data []byte) uint64 {
	return HashWithSeed(data, RapidSeed)
}

// HashWithSeed computes the rapidhash of the input data with custom seed
func HashWithSeed(data []byte, seed uint64) uint64 {
	return HashWithSeedAndSecret(data, seed, rapidSecret)
}

// HashWithSeedAndSecret computes the rapidhash with custom seed and secret
func HashWithSeedAndSecret(data []byte, seed uint64, secret [3]uint64) uint64 {
	length := uint64(len(data))

	// Initialize with seed, secret, and length
	seed ^= rapidMix(seed^secret[0], secret[1]) ^ length

	var a, b uint64

	if length <= 16 {
		if length >= 4 {
			// Read first and last 32 bits (may overlap)
			delta := (length & 24) >> (length >> 3)
			a = (read32(data) << 32) | read32(data[length-4:])
			b = (read32(data[delta:]) << 32) | read32(data[length-4-delta:])
		} else if length > 0 {
			// For very small inputs (1-3 bytes)
			a = uint64(data[0])
			a |= uint64(data[length>>1]) << 8
			a |= uint64(data[length-1]) << 16
			b = 0
		} else {
			// Empty input
			a, b = 0, 0
		}
	} else {
		i := length
		if i > 48 {
			// For longer inputs, process in 48-byte chunks
			see1, see2 := seed, seed

			for i > 48 {
				seed = rapidMix(read64(data[length-i:])^secret[0], read64(data[length-i+8:])^seed)
				see1 = rapidMix(read64(data[length-i+16:])^secret[1], read64(data[length-i+24:])^see1)
				see2 = rapidMix(read64(data[length-i+32:])^secret[2], read64(data[length-i+40:])^see2)
				i -= 48
			}
			seed ^= see1 ^ see2
		}

		if i > 16 {
			seed = rapidMix(read64(data[length-i:])^secret[2], read64(data[length-i+8:])^seed^secret[1])
			if i > 32 {
				seed = rapidMix(read64(data[length-i+16:])^secret[2], read64(data[length-i+24:])^seed)
			}
		}

		// Read final 16 bytes
		a = read64(data[length-16:])
		b = read64(data[length-8:])
	}

	// Final mixing
	a ^= secret[1]
	b ^= seed
	rapidMul(&a, &b)

	return rapidMix(a^secret[0]^length, b^secret[2])
}

// String hashes a string directly without allocation
func String(s string) uint64 {
	return StringWithSeed(s, RapidSeed)
}

// StringWithSeed hashes a string with custom seed
func StringWithSeed(s string, seed uint64) uint64 {
	// Convert string to []byte without allocation using unsafe
	data := *(*[]byte)(unsafe.Pointer(&struct {
		string
		int
	}{s, len(s)}))

	return HashWithSeed(data, seed)
}

// Uint64 hashes a single uint64 value
func Uint64(value uint64) uint64 {
	return Uint64WithSeed(value, RapidSeed)
}

// Uint64WithSeed hashes a single uint64 value with custom seed
func Uint64WithSeed(value, seed uint64) uint64 {
	// For a single 64-bit value, we can optimize
	seed ^= rapidMix(seed^rapidSecret[0], rapidSecret[1]) ^ 8 // length = 8

	a := value ^ rapidSecret[1]
	b := seed
	rapidMul(&a, &b)

	return rapidMix(a^rapidSecret[0]^8, b^rapidSecret[2])
}

// Uint32 hashes a single uint32 value
func Uint32(value uint32) uint64 {
	return Uint32WithSeed(value, RapidSeed)
}

// Uint32WithSeed hashes a single uint32 value with custom seed
func Uint32WithSeed(value uint32, seed uint64) uint64 {
	seed ^= rapidMix(seed^rapidSecret[0], rapidSecret[1]) ^ 4 // length = 4

	a := uint64(value) ^ rapidSecret[1]
	b := seed
	rapidMul(&a, &b)

	return rapidMix(a^rapidSecret[0]^4, b^rapidSecret[2])
}

// Hasher provides a hash.Hash64 compatible interface
type Hasher struct {
	seed   uint64
	secret [3]uint64
	buf    []byte
}

// NewHasher creates a new Hasher with default seed and secret
func NewHasher() *Hasher {
	return &Hasher{
		seed:   RapidSeed,
		secret: rapidSecret,
		buf:    make([]byte, 0, 64),
	}
}

// NewHasherWithSeed creates a new Hasher with custom seed
func NewHasherWithSeed(seed uint64) *Hasher {
	return &Hasher{
		seed:   seed,
		secret: rapidSecret,
		buf:    make([]byte, 0, 64),
	}
}

// Write implements io.Writer
func (h *Hasher) Write(p []byte) (n int, err error) {
	h.buf = append(h.buf, p...)
	return len(p), nil
}

// Sum64 returns the 64-bit hash
func (h *Hasher) Sum64() uint64 {
	return HashWithSeedAndSecret(h.buf, h.seed, h.secret)
}

// Sum appends the hash to b and returns the result
func (h *Hasher) Sum(b []byte) []byte {
	hash := h.Sum64()
	return binary.LittleEndian.AppendUint64(b, hash)
}

// Reset resets the hasher to its initial state
func (h *Hasher) Reset() {
	h.buf = h.buf[:0]
}

// Size returns the hash size in bytes (8 for 64-bit)
func (h *Hasher) Size() int {
	return 8
}

// BlockSize returns the block size (rapidhash doesn't have a fixed block size)
func (h *Hasher) BlockSize() int {
	return 48 // The chunk size used for long inputs
}
