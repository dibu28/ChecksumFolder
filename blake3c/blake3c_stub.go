//go:build !cgo || (!amd64 && !arm64 && !arm)

package blake3c

import "github.com/zeebo/blake3"

// Hasher wraps the pure-Go blake3 Hasher to match the cgo implementation API.
type Hasher struct{ h *blake3.Hasher }

func BLAKE3Init() *Hasher {
	return &Hasher{h: blake3.New()}
}

func BLAKE3Update(h *Hasher, b []byte) {
	if len(b) == 0 {
		return
	}
	h.h.Write(b)
}

func BLAKE3Finalize(h *Hasher) [32]byte {
	var out [32]byte
	copy(out[:], h.h.Sum(nil))
	return out
}

func (h *Hasher) Reset()                      { h.h.Reset() }
func (h *Hasher) Write(p []byte) (int, error) { return h.h.Write(p) }
func (h *Hasher) Sum(b []byte) []byte         { return h.h.Sum(b) }
func (h *Hasher) Size() int                   { return 32 }
func (h *Hasher) BlockSize() int              { return 64 }
