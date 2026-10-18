package fakerjs

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/iamramtin/go-fakerjs/internal/wasi"
	"github.com/iamramtin/go-fakerjs/internal/wasm"
)

// Module status codes, the first byte after the length in a call() result. Mirrors wasm/harness.c.
const (
	stOK             = 0
	stUnknownMethod  = 1
	stJSException    = 2
	stInvalidArg     = 3
	stInterrupted    = 4
	stOutOfMemory    = 5
	stResultTooLarge = 6
	stInternal       = 7
	stInvalidMethod  = 8
)

// maxPathBytes mirrors MAX_PATH_BYTES in the module.
const maxPathBytes = 256

// Engine runs Faker.js methods in a private pre-initialised Wasm instance.
//
// A non-zero seed makes the generated sequence reproducible; a zero seed leaves Faker.js on its own
// non-deterministic default. The instance is built lazily on the first call and released by Close, which
// is terminal. An Engine is safe for concurrent use; calls are serialised by an internal mutex.
type Engine struct {
	mu       sync.Mutex
	cfg      config
	seed     int64
	inst     *instance
	closed   bool
	poisoned bool
}

// NewEngine returns an Engine that seeds Faker.js with seed on its first call. It does no work until then.
func NewEngine(seed int64, opts ...Option) *Engine {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	return &Engine{seed: seed, cfg: cfg}
}

// instance is one running module.
type instance struct {
	m    *wasm.Module
	host *wasi.Host
}

// imports adapts the shared host to the fakerjs import module.
type imports struct {
	host *wasi.Host
	hook func()
}

func (i imports) Xmonotonic_ns() int64 {
	if i.hook != nil {
		i.hook()
	}

	return i.host.MonotonicNS()
}

func (in *instance) mem() []byte { return *in.m.Xmemory().Slice() }

// newInstance instantiates the snapshot and applies entropy, seed and reference date, in that order.
func newInstance(cfg config, seed int64) (*instance, error) {
	host := wasi.New(cfg.clock)
	in := &instance{host: host}
	in.m = wasm.New(imports{host, cfg.hostHook}, host)

	var entropy [32]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, fmt.Errorf("fakerjs: reading entropy: %w", err)
	}

	p, err := in.write(entropy[:], nil)
	if err != nil {
		return nil, err
	}

	in.m.Xreseed_math_random(p, int32(len(entropy)))
	in.m.Xfree(p)

	if seed != 0 {
		// Same conversion V8 applies to the literal in faker.seed(<int64>): round to nearest even.
		if in.m.Xset_seed(float64(seed)) != 0 {
			return nil, fmt.Errorf("fakerjs: seeding failed: %s", in.lastError())
		}
	} else if in.m.Xseed_random() != 0 {
		return nil, fmt.Errorf("fakerjs: random seeding failed: %s", in.lastError())
	}

	if !cfg.refDate.IsZero() {
		if in.m.Xset_ref_date(float64(cfg.refDate.UnixMilli())) != 0 {
			return nil, fmt.Errorf("fakerjs: setting reference date failed: %s", in.lastError())
		}
	}

	return in, nil
}

func (in *instance) lastError() string {
	p := in.m.Xlast_error()
	mem := in.mem()
	n := binary.LittleEndian.Uint32(mem[p:])

	return string(mem[p+4 : p+4+int32(n)])
}

// write copies a then b into one fresh module allocation (NUL-terminated) and returns its address.
func (in *instance) write(a, b []byte) (int32, error) {
	p := in.m.Xalloc(int32(len(a) + len(b)))
	if p == 0 {
		return 0, ErrOutOfMemory
	}

	mem := in.mem()
	copy(mem[p:], a)
	copy(mem[int(p)+len(a):], b)

	return p, nil
}

// Generate runs method and returns its result coerced like JavaScript's String(result).
//
// A nil arg calls the method bare, e.g. Generate("person.fullName", nil). Anything else is marshalled to
// JSON and passed as the single argument, e.g. Generate("string.alpha", map[string]any{"length": 5}).
func (e *Engine) Generate(method string, arg any) (string, error) {
	out, err := e.GenerateN(method, arg, 1)
	if err != nil {
		return "", err
	}

	return out[0], nil
}

// GenerateN returns n consecutive results of method in one boundary crossing. The results equal those of
// n successive Generate calls with the same arguments.
func (e *Engine) GenerateN(method string, arg any, n int) ([]string, error) {
	if method == "" {
		return nil, &Error{Class: ClassInvalidMethod, Msg: "faker method cannot be empty"}
	}

	if !ValidMethod(method) || len(method) > maxPathBytes {
		return nil, &Error{Class: ClassInvalidMethod, Msg: fmt.Sprintf("invalid faker method format: %q (expected format: module.method)", method)}
	}

	if n < 0 {
		return nil, fmt.Errorf("fakerjs: negative count %d", n)
	}

	var argJSON []byte

	if arg != nil {
		var err error

		argJSON, err = json.Marshal(arg)
		if err != nil {
			return nil, &Error{Class: ClassInvalidArgument, Method: method, Msg: "failed to marshal faker argument: " + err.Error(), Err: err}
		}
	}

	return e.generateJSON(method, argJSON, n)
}

// generateJSON is GenerateN after argument marshalling. argJSON is passed to the module verbatim, so tests
// can feed it bytes encoding/json would never produce.
func (e *Engine) generateJSON(method string, argJSON []byte, n int) ([]string, error) {
	if len(argJSON) > e.cfg.maxArgBytes {
		return nil, &Error{Class: ClassInvalidArgument, Method: method, Err: ErrArgTooLarge}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	switch {
	case e.closed:
		return nil, &Error{Class: ClassEngine, Err: ErrClosed}
	case e.poisoned:
		return nil, &Error{Class: ClassEngine, Err: ErrPoisoned}
	}

	if e.inst == nil {
		in, err := e.start()
		if err != nil {
			return nil, err
		}

		e.inst = in
	}

	if n == 0 {
		return []string{}, nil
	}

	return e.call(method, argJSON, n)
}

// start builds the instance, converting a panic during start-up into a poisoned engine.
func (e *Engine) start() (in *instance, err error) {
	defer func() {
		if r := recover(); r != nil {
			e.poisoned = true
			in, err = nil, &Error{Class: ClassEngine, Msg: fmt.Sprintf("engine start-up panicked: %v", r), Err: ErrPoisoned}
		}
	}()

	in, err = newInstance(e.cfg, e.seed)
	if err != nil {
		return nil, &Error{Class: ClassEngine, Msg: err.Error()}
	}

	return in, nil
}

// call performs one module call. Callers hold e.mu.
func (e *Engine) call(method string, argJSON []byte, n int) (out []string, err error) {
	in := e.inst

	defer func() {
		if r := recover(); r != nil {
			e.poisoned = true
			out, err = nil, &Error{Class: ClassEngine, Method: method, Msg: fmt.Sprintf("engine panicked during %s: %v", method, r), Err: ErrPoisoned}
		}
	}()

	base, werr := in.write([]byte(method), argJSON)
	if werr != nil {
		return nil, &Error{Class: ClassResource, Method: method, Err: werr}
	}
	defer in.m.Xfree(base)

	if e.cfg.timeout > 0 {
		in.m.Xset_deadline(in.host.MonotonicNS() + int64(e.cfg.timeout))
		defer in.m.Xset_deadline(0)
	}

	res := in.m.Xcall(base, int32(len(method)), base+int32(len(method)), int32(len(argJSON)), int32(n))
	if res == 0 {
		return nil, &Error{Class: ClassResource, Method: method, Err: ErrOutOfMemory}
	}
	defer in.m.Xfree(res)

	mem := in.mem()
	payloadLen := int(binary.LittleEndian.Uint32(mem[res:]))
	status := mem[res+4]
	payload := mem[int(res)+5 : int(res)+5+payloadLen]

	if status != stOK {
		return nil, classify(method, status, string(payload))
	}

	out = make([]string, 0, n)

	for len(payload) > 0 {
		if len(payload) < 4 {
			return nil, e.poison(method, "truncated result buffer")
		}

		l := int(binary.LittleEndian.Uint32(payload))
		if 4+l > len(payload) {
			return nil, e.poison(method, "corrupt result buffer")
		}

		s := string(payload[4 : 4+l])
		if !utf8.ValidString(s) {
			s = strings.ToValidUTF8(s, "�")
		}

		out = append(out, s)
		payload = payload[4+l:]
	}

	if len(out) != n {
		return nil, e.poison(method, fmt.Sprintf("wasm module returned %d results, want %d", len(out), n))
	}

	return out, nil
}

func (e *Engine) poison(method, why string) error {
	e.poisoned = true
	return &Error{Class: ClassEngine, Method: method, Msg: why, Err: ErrPoisoned}
}

func classify(method string, status byte, msg string) error {
	switch status {
	case stInvalidMethod:
		return &Error{Class: ClassInvalidMethod, Method: method, Msg: fmt.Sprintf("invalid faker method format: %q (expected format: module.method)", method)}
	case stUnknownMethod:
		return &Error{Class: ClassUnknownMethod, Method: method, Msg: fmt.Sprintf("unknown faker method %q", method), Err: ErrUnknownMethod}
	case stJSException:
		return &Error{Class: ClassJSException, Method: method, Msg: msg}
	case stInvalidArg:
		return &Error{Class: ClassInvalidArgument, Method: method, Msg: "invalid faker argument: " + msg}
	case stInterrupted:
		return &Error{Class: ClassResource, Method: method, Err: ErrDeadline}
	case stOutOfMemory:
		return &Error{Class: ClassResource, Method: method, Err: ErrOutOfMemory}
	case stResultTooLarge:
		return &Error{Class: ClassResource, Method: method, Err: ErrResultTooLarge}
	}

	return &Error{Class: ClassEngine, Method: method, Msg: "wasm module internal error: " + msg}
}

// Close releases the instance. It is safe to call more than once and on an engine that never started.
//
// A closed engine stays closed: every later call fails with [ErrClosed] instead of building a fresh
// instance whose sequence would restart from the seed.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.closed = true
	e.inst = nil
}

// Poisoned reports whether a trap or panic has left the engine unusable.
func (e *Engine) Poisoned() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.poisoned
}

// IsClass reports whether err is an [*Error] of the given class.
func IsClass(err error, class string) bool {
	var fe *Error
	return errors.As(err, &fe) && fe.Class == class
}
