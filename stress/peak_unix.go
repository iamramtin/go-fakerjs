//go:build unix

package stress

import (
	"runtime"
	"syscall"
)

// PeakRSS returns the process's peak resident set size in bytes (ru_maxrss).
func PeakRSS() uint64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}

	if runtime.GOOS == "darwin" {
		return uint64(ru.Maxrss) // bytes
	}

	return uint64(ru.Maxrss) * 1024 // KiB
}
