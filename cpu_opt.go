package main

import (
	stdsha256 "crypto/sha256"
	"runtime"

	"github.com/klauspost/cpuid/v2"
	sha256 "github.com/minio/sha256-simd"
	"hash"
)

var sha256HashFn func() hash.Hash = sha256.New

func init() {
	if runtime.GOARCH == "arm" {
		cpuid.DetectARM()
		if !cpuid.CPU.Has(cpuid.ASIMD) {
			sha256HashFn = stdsha256.New
		}
	}
}
