// Package stress holds the robustness tests: GC pressure, leaks, isolation, resource bombs, deadlines, fuzzing.
package stress

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// RSS returns the resident set size of the process in bytes (0 if unknown).
func RSS() uint64 {
	if b, err := os.ReadFile("/proc/self/statm"); err == nil {
		f := strings.Fields(string(b))
		if len(f) > 1 {
			pages, _ := strconv.ParseUint(f[1], 10, 64)
			return pages * uint64(os.Getpagesize())
		}
	}
	// macOS and BSDs: ask ps (RSS in KiB).
	if out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output(); err == nil {
		kb, _ := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
		return kb * 1024
	}

	return 0
}
