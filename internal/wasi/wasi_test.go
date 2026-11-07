package wasi

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

// fakeMemory stands in for the module's linear memory.
type fakeMemory struct{ b []byte }

func (m *fakeMemory) Slice() *[]byte              { return &m.b }
func (m *fakeMemory) Grow(delta, max int64) int64 { return -1 }

type fakeModule struct{ mem *fakeMemory }

func (f fakeModule) Xmemory() memory { return f.mem }

// newHost returns a Host over size bytes of memory, with the wall clock fixed at now.
func newHost(size int, now time.Time) (*Host, []byte) {
	mem := &fakeMemory{b: make([]byte, size)}
	h := New(func() time.Time { return now })
	h.Init(fakeModule{mem})

	return h, mem.b
}

// putIovec writes one (base, len) iovec at at.
func putIovec(mem []byte, at, base, n int) {
	binary.LittleEndian.PutUint32(mem[at:], uint32(base))
	binary.LittleEndian.PutUint32(mem[at+4:], uint32(n))
}

func TestClockTimeGet(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h, mem := newHost(64, now)

	if got := h.Xclock_time_get(clockRealtime, 0, 8); got != errnoSuccess {
		t.Fatalf("realtime: errno %d", got)
	}

	if got := binary.LittleEndian.Uint64(mem[8:]); got != uint64(now.UnixNano()) {
		t.Errorf("realtime wrote %d, want the injected clock's %d", got, now.UnixNano())
	}

	for _, id := range []int32{clockMonotonic, 2, 3} {
		if got := h.Xclock_time_get(id, 0, 16); got != errnoSuccess {
			t.Errorf("clock %d: errno %d", id, got)
		}
	}

	if got := h.Xclock_time_get(4, 0, 8); got != errnoInval {
		t.Errorf("unknown clock: errno %d, want %d", got, errnoInval)
	}

	for _, ptr := range []int32{-1, 57} { // 57+8 runs past the 64-byte memory
		if got := h.Xclock_time_get(clockRealtime, 0, ptr); got != errnoFault {
			t.Errorf("result pointer %d: errno %d, want %d", ptr, got, errnoFault)
		}
	}
}

func TestFdWriteCollectsStdoutAndStderr(t *testing.T) {
	h, mem := newHost(256, time.Time{})
	copy(mem[100:], "first ")
	copy(mem[120:], "second")
	putIovec(mem, 0, 100, 6)
	putIovec(mem, 8, 120, 6)

	if got := h.Xfd_write(2, 0, 2, 32); got != errnoSuccess {
		t.Fatalf("fd 2: errno %d", got)
	}

	if got := binary.LittleEndian.Uint32(mem[32:]); got != 12 {
		t.Errorf("nwritten %d, want 12", got)
	}

	if got := h.Xfd_write(1, 8, 1, 32); got != errnoSuccess {
		t.Fatalf("fd 1: errno %d", got)
	}

	if got, want := h.Stderr(), "first secondsecond"; got != want {
		t.Errorf("Stderr() = %q, want %q", got, want)
	}
}

func TestFdWriteRejectsBadInput(t *testing.T) {
	h, mem := newHost(64, time.Time{})
	putIovec(mem, 0, 40, 30) // 40+30 runs past the 64-byte memory

	cases := []struct {
		name                       string
		fd, iovs, iovsLen, written int32
		want                       int32
	}{
		{"fd other than stdout or stderr", 3, 0, 0, 16, errnoBadf},
		{"iovec outside memory", 2, 60, 1, 16, errnoFault},
		{"data outside memory", 2, 0, 1, 16, errnoFault},
		{"nwritten outside memory", 2, 0, 0, 62, errnoFault},
	}
	for _, c := range cases {
		if got := h.Xfd_write(c.fd, c.iovs, c.iovsLen, c.written); got != c.want {
			t.Errorf("%s: errno %d, want %d", c.name, got, c.want)
		}
	}

	if got := h.Stderr(); got != "" {
		t.Errorf("rejected writes kept %q", got)
	}
}

// The retained output stops at StderrLimit, but nwritten still reports every byte, so the module's libc
// never retries a write the host chose to drop.
func TestFdWriteTruncatesAtLimit(t *testing.T) {
	const n = StderrLimit + 10

	h, mem := newHost(n+64, time.Time{})
	copy(mem[64:], strings.Repeat("x", n))
	putIovec(mem, 0, 64, n)

	if got := h.Xfd_write(2, 0, 1, 16); got != errnoSuccess {
		t.Fatalf("errno %d", got)
	}

	if got := binary.LittleEndian.Uint32(mem[16:]); got != n {
		t.Errorf("nwritten %d, want %d", got, n)
	}

	if got := len(h.Stderr()); got != StderrLimit {
		t.Errorf("kept %d bytes, want %d", got, StderrLimit)
	}

	if got := h.Xfd_write(2, 0, 1, 16); got != errnoSuccess || len(h.Stderr()) != StderrLimit {
		t.Errorf("a write after the limit: errno %d, kept %d bytes", got, len(h.Stderr()))
	}
}

func TestUnsupportedCallsReportNosys(t *testing.T) {
	h, _ := newHost(8, time.Time{})

	for name, got := range map[string]int32{
		"fd_close":      h.Xfd_close(1),
		"fd_fdstat_get": h.Xfd_fdstat_get(1, 0),
		"fd_seek":       h.Xfd_seek(1, 0, 0, 0),
	} {
		if got != errnoNosys {
			t.Errorf("%s: errno %d, want %d", name, got, errnoNosys)
		}
	}
}
