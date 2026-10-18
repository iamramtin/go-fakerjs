// Package wasi is the minimal WASI preview1 host the Faker module still imports after pre-initialisation:
// clocks and writes to stdout/stderr. Every other call reports ENOSYS. The module has no filesystem, no
// environment, no network and no entropy source of its own.
package wasi

import (
	"encoding/binary"
	"sync"
	"time"
)

// WASI errno values used by the shim.
const (
	errnoSuccess = 0
	errnoBadf    = 8
	errnoInval   = 28
	errnoNosys   = 52
	errnoFault   = 21
)

// WASI clock ids.
const (
	clockRealtime  = 0
	clockMonotonic = 1
)

// StderrLimit bounds the bytes retained from module writes to stdout/stderr.
const StderrLimit = 64 << 10

type memory = interface {
	Slice() *[]byte
	Grow(delta, max int64) int64
}

// Host implements the generated wasi_snapshot_preview1 import interface.
type Host struct {
	// Clock supplies wall-clock time. It is the only seam tests use to fake time.
	Clock func() time.Time

	start time.Time
	mem   func() []byte

	mu     sync.Mutex
	stderr []byte
}

// New returns a Host whose monotonic clock starts now. A nil clock means time.Now.
func New(clock func() time.Time) *Host {
	if clock == nil {
		clock = time.Now
	}

	return &Host{Clock: clock, start: time.Now()}
}

// Init receives the instantiated module; the generated code calls it from New.
func (h *Host) Init(m any) {
	if mm, ok := m.(interface{ Xmemory() memory }); ok {
		mem := mm.Xmemory()
		h.mem = func() []byte { return *mem.Slice() }
	}
}

// MonotonicNS is the monotonic clock shared with the module's interrupt handler.
func (h *Host) MonotonicNS() int64 { return int64(time.Since(h.start)) }

// Stderr returns what the module wrote to stdout/stderr, truncated to StderrLimit bytes.
func (h *Host) Stderr() string {
	h.mu.Lock()
	defer h.mu.Unlock()

	return string(h.stderr)
}

// Xclock_time_get implements clock_time_get.
func (h *Host) Xclock_time_get(id int32, precision int64, resultPtr int32) int32 {
	var ns uint64

	switch id {
	case clockRealtime:
		ns = uint64(h.Clock().UnixNano())
	case clockMonotonic, 2, 3: // process/thread CPU time are approximated by elapsed time
		ns = uint64(h.MonotonicNS())
	default:
		return errnoInval
	}

	mem := h.mem()
	if int64(resultPtr) < 0 || int64(resultPtr)+8 > int64(len(mem)) {
		return errnoFault
	}

	binary.LittleEndian.PutUint64(mem[resultPtr:], ns)

	return errnoSuccess
}

// Xfd_write implements fd_write for fds 1 and 2 only.
func (h *Host) Xfd_write(fd, iovs, iovsLen, nwrittenPtr int32) int32 {
	if fd != 1 && fd != 2 {
		return errnoBadf
	}

	mem := h.mem()

	var total uint32

	h.mu.Lock()
	defer h.mu.Unlock()

	for i := range iovsLen {
		at := int64(iovs) + int64(i)*8
		if at < 0 || at+8 > int64(len(mem)) {
			return errnoFault
		}

		base := int64(binary.LittleEndian.Uint32(mem[at:]))

		n := int64(binary.LittleEndian.Uint32(mem[at+4:]))
		if base+n > int64(len(mem)) {
			return errnoFault
		}

		if room := StderrLimit - len(h.stderr); room > 0 {
			h.stderr = append(h.stderr, mem[base:base+min(n, int64(room))]...)
		}

		total += uint32(n)
	}

	if int64(nwrittenPtr) < 0 || int64(nwrittenPtr)+4 > int64(len(mem)) {
		return errnoFault
	}

	binary.LittleEndian.PutUint32(mem[nwrittenPtr:], total)

	return errnoSuccess
}

// Xfd_close implements fd_close: unsupported.
func (h *Host) Xfd_close(fd int32) int32 { return errnoNosys }

// Xfd_fdstat_get implements fd_fdstat_get: unsupported.
func (h *Host) Xfd_fdstat_get(fd, ptr int32) int32 { return errnoNosys }

// Xfd_seek implements fd_seek: unsupported.
func (h *Host) Xfd_seek(fd int32, offset int64, whence, ptr int32) int32 { return errnoNosys }
