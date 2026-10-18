// Package qjsbase is Baseline B: the real Faker.js bundle on fastschema/qjs (QuickJS-NG compiled to Wasm,
// interpreted/compiled by wazero), exposed through the same Generate/GenerateN/Close shape as the native engine.
package qjsbase

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"

	"github.com/fastschema/qjs"
)

var methodRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*\.[a-zA-Z][a-zA-Z0-9]*$`)

// ErrClosed is returned after Close.
var ErrClosed = errors.New("engine is closed")

// Error carries an engine-independent class so the conformance harness can compare error classes.
type Error struct {
	Class string
	Msg   string
}

func (e *Error) Error() string      { return e.Msg }
func (e *Error) ErrorClass() string { return e.Class }

// driver is evaluated once after the bundle. It mirrors the native engine's contract: a precomputed
// allowlist of module.method names, one JSON argument, String() coercion.
const driver = `
globalThis.__allow = (function () {
  var skip = Object.getOwnPropertyNames(Object.prototype);
  var set = Object.create(null);
  Reflect.ownKeys(faker).forEach(function (k) {
    var mod = faker[k];
    if (typeof k !== 'string' || k.charAt(0) === '_' || !mod || typeof mod !== 'object' || mod.faker !== faker) return;
    var o = mod;
    while (o && o !== Object.prototype) {
      Reflect.ownKeys(o).forEach(function (n) {
        if (typeof n !== 'string' || n.charAt(0) === '_' || n === 'constructor' || skip.indexOf(n) >= 0) return;
        var d = Object.getOwnPropertyDescriptor(o, n);
        if (d && typeof d.value === 'function') set[k + '.' + n] = true;
      });
      o = Object.getPrototypeOf(o);
    }
  });
  return set;
})();
globalThis.__gen = function (m, aj, n) {
  if (!__allow[m]) return JSON.stringify({ c: 'unknown-method', m: 'unknown faker method: ' + m });
  var p = m.split('.'), mod = faker[p[0]], out = [];
  try {
    var args = aj === '' ? [] : [JSON.parse(aj)];
    for (var i = 0; i < n; i++) out.push(String(mod[p[1]].apply(mod, args)));
  } catch (e) {
    return JSON.stringify({ c: 'js-exception', m: String(e && e.message) });
  }
  return JSON.stringify({ r: out });
};
`

// Engine is one QuickJS runtime holding the Faker bundle.
type Engine struct {
	mu     sync.Mutex
	rt     *qjs.Runtime
	closed bool
}

// BundlePath returns the en-only bundle path for the repository this package lives in.
func BundlePath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "bundle", "faker.en.min.js")
}

// New builds an engine; seed 0 keeps Faker's own random seed. refDate is applied when non-zero.
func New(seed int64, refDate time.Time) (*Engine, error) {
	bundle, err := os.ReadFile(BundlePath())
	if err != nil {
		return nil, err
	}

	rt, err := qjs.New()
	if err != nil {
		return nil, err
	}

	e := &Engine{rt: rt}

	ctx := rt.Context()
	if err := eval(ctx, "faker.js", string(bundle)); err != nil {
		rt.Close()
		return nil, err
	}

	if err := eval(ctx, "driver.js", driver); err != nil {
		rt.Close()
		return nil, err
	}

	if seed != 0 {
		if err := eval(ctx, "seed.js", fmt.Sprintf("faker.seed(%d)", seed)); err != nil {
			rt.Close()
			return nil, err
		}
	}

	if !refDate.IsZero() {
		if err := eval(ctx, "ref.js", fmt.Sprintf("faker.setDefaultRefDate(%q)", refDate.UTC().Format("2006-01-02T15:04:05.000Z"))); err != nil {
			rt.Close()
			return nil, err
		}
	}

	return e, nil
}

func eval(ctx *qjs.Context, name, code string) error {
	v, err := ctx.Eval(name, qjs.Code(code))
	if err != nil {
		return err
	}

	v.Free()

	return nil
}

// Generate implements the native contract on QuickJS.
func (e *Engine) Generate(method string, arg any) (string, error) {
	r, err := e.GenerateN(method, arg, 1)
	if err != nil {
		return "", err
	}

	return r[0], nil
}

// GenerateN returns n consecutive results in a single boundary crossing.
func (e *Engine) GenerateN(method string, arg any, n int) ([]string, error) {
	if method == "" {
		return nil, &Error{ClassInvalidMethod, "faker method cannot be empty"}
	}

	if !methodRE.MatchString(method) {
		return nil, &Error{ClassInvalidMethod, fmt.Sprintf("invalid faker method format: %q", method)}
	}

	aj := ""

	if arg != nil {
		b, err := json.Marshal(arg)
		if err != nil {
			return nil, err
		}

		aj = string(b)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil, ErrClosed
	}

	ctx := e.rt.Context()

	vm, va, vn := ctx.NewString(method), ctx.NewString(aj), ctx.NewInt32(int32(n))
	defer vm.Free()
	defer va.Free()
	defer vn.Free()

	g := ctx.Global()

	res, err := g.InvokeJS("__gen", vm, va, vn)
	if err != nil {
		return nil, &Error{ClassJSException, err.Error()}
	}
	defer res.Free()

	var out struct {
		R []string `json:"r"`
		C string   `json:"c"`
		M string   `json:"m"`
	}
	if err := json.Unmarshal([]byte(res.String()), &out); err != nil {
		return nil, err
	}

	if out.C != "" {
		return nil, &Error{out.C, out.M}
	}

	return out.R, nil
}

// Close releases the runtime; it is idempotent.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return
	}

	e.closed = true
	e.rt.Close()
}

// Error classes, duplicated here to avoid importing the harness from the engine.
const (
	ClassInvalidMethod = "invalid-method"
	ClassJSException   = "js-exception"
)

// Eval runs arbitrary JS and returns String(result). It exists for semantic probes only and is not part
// of the Generate contract.
func (e *Engine) Eval(code string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	v, err := e.rt.Context().Eval("probe.js", qjs.Code(code))
	if err != nil {
		return "", err
	}
	defer v.Free()

	return v.String(), nil
}
