package main

import (
	"runtime"

	cpuid "github.com/klauspost/cpuid/v2"
)

var useStdSHA256 bool
var useBlake3C bool

func init() {
	// On ARM systems some features require explicit detection
	if runtime.GOARCH == "arm64" || runtime.GOARCH == "arm" {
		cpuid.DetectARM()
	}

	// Fallback to the standard crypto/sha256 if we lack SIMD features
	switch runtime.GOARCH {
	case "amd64", "386":
		if !cpuid.CPU.Supports(cpuid.SSE2) {
			useStdSHA256 = true
		}
	case "arm64", "arm":
		if !cpuid.CPU.Supports(cpuid.ASIMD) {
			useStdSHA256 = true
		} else {
			useBlake3C = true
		}
	default:
		// Unknown architecture, use conservative default
		useStdSHA256 = true
	}
}
