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

// Split is the resident memory of the process by kind, in bytes. Fields are zero where /proc does not report them.
type Split struct {
	Anon, File uint64 // RssAnon and RssFile from /proc/self/status
}

// ReadSplit reads the resident memory split. Anon is memory the process owns; File is file-backed pages, such as
// the test binary's code and mapped data.
func ReadSplit() Split {
	var s Split

	if b, err := os.ReadFile("/proc/self/status"); err == nil {
		for line := range strings.SplitSeq(string(b), "\n") {
			switch name, v, _ := strings.Cut(line, ":"); name {
			case "RssAnon":
				s.Anon = kibField(v)
			case "RssFile":
				s.File = kibField(v)
			}
		}
	}

	return s
}

// kibField parses a "  1234 kB" value into bytes.
func kibField(v string) uint64 {
	f := strings.Fields(v)
	if len(f) == 0 {
		return 0
	}

	kb, _ := strconv.ParseUint(f[0], 10, 64)

	return kb * 1024
}
