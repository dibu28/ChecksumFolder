package main

import (
	"runtime"

	cpuid "github.com/klauspost/cpuid/v2"
)

var useStdSHA256 bool
var useBlake3C bool
var useWyhashC bool
var useRapidhashC bool

func init() {
	// On ARM systems some features require explicit detection.
	// Only attempt this when user space access to the CPU ID registers is
	// allowed; otherwise attempting detection may trigger an illegal
	// instruction fault on older kernels.
	if (runtime.GOARCH == "arm64" || runtime.GOARCH == "arm") && cpuid.CPU.Supports(cpuid.ARMCPUID) {
		cpuid.DetectARM()
	}

	// Fallback to the standard crypto/sha256 if we lack SIMD features
	switch runtime.GOARCH {
	case "amd64", "386":
		if cpuid.CPU.Supports(cpuid.SSE2) {
			useWyhashC = true
			useRapidhashC = true
		} else {
			useStdSHA256 = true
		}
	case "arm64":
		if cpuid.CPU.Supports(cpuid.ASIMD) {
			useBlake3C = true
			useWyhashC = true
			useRapidhashC = true
		} else {
			useStdSHA256 = true
		}
	case "arm":
		useStdSHA256 = true
	default:
		// Unknown architecture, use conservative default
		useStdSHA256 = true
	}
}
