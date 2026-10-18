//go:build v8go

// UNVERIFIED: this wrapper was written but never compiled or run in the spike environment, because the
// prebuilt V8 static library (github.com/tommie/v8go/deps/*) could not be downloaded there. It mirrors the
// calls a V8 embedding makes (NewContext/RunScript/Close). Build with:
//
//	make bench-v8go
package bench

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/tommie/v8go"

	"github.com/iamramtin/go-fakerjs"
	"github.com/iamramtin/go-fakerjs/conformance"
)

type v8Engine struct {
	mu  sync.Mutex
	vm  *v8go.Context
	iso *v8go.Isolate
}

func newV8(seed int64) (conformance.Engine, error) {
	bundle, err := os.ReadFile(filepath.Join(conformance.Root(), "bundle", "faker.en.min.js"))
	if err != nil {
		return nil, err
	}

	iso := v8go.NewIsolate()

	vm := v8go.NewContext(iso)
	if _, err := vm.RunScript(string(bundle), "faker.min.js"); err != nil {
		return nil, err
	}

	if seed != 0 {
		if _, err := vm.RunScript(fmt.Sprintf("faker.seed(%d)", seed), "seed"); err != nil {
			return nil, err
		}
	}

	if _, err := vm.RunScript(fmt.Sprintf("faker.setDefaultRefDate(%q)", benchRef.Format("2006-01-02T15:04:05.000Z")), "ref"); err != nil {
		return nil, err
	}

	return &v8Engine{vm: vm, iso: iso}, nil
}

func (e *v8Engine) Generate(method string, arg any) (string, error) {
	if !fakerjs.ValidMethod(method) {
		return "", errors.New("invalid faker method format")
	}

	script := fmt.Sprintf("faker.%s()", method)

	if arg != nil {
		b, _ := json.Marshal(arg)
		script = fmt.Sprintf("faker.%s(%s)", method, b)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	v, err := e.vm.RunScript(script, "faker-exec")
	if err != nil {
		return "", err
	}

	return v.String(), nil
}

func (e *v8Engine) GenerateN(method string, arg any, n int) ([]string, error) {
	out := make([]string, 0, n)
	for range n {
		s, err := e.Generate(method, arg)
		if err != nil {
			return nil, err
		}

		out = append(out, s)
	}

	return out, nil
}

func (e *v8Engine) Close() { e.vm.Close(); e.iso.Dispose() }

// v8Precompiled compiles each argument-less method's script once (v8go.UnboundScript) and only runs it per
// call. It is the best case for an embedding that keeps a one-call-per-value shape.
type v8Precompiled struct {
	*v8Engine
	scripts map[string]*v8go.UnboundScript
}

func newV8Precompiled(seed int64) (conformance.Engine, error) {
	e, err := newV8(seed)
	if err != nil {
		return nil, err
	}

	return &v8Precompiled{v8Engine: e.(*v8Engine), scripts: map[string]*v8go.UnboundScript{}}, nil
}

func (e *v8Precompiled) Generate(method string, arg any) (string, error) {
	if arg != nil || !fakerjs.ValidMethod(method) {
		return e.v8Engine.Generate(method, arg)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	us, ok := e.scripts[method]
	if !ok {
		var err error

		us, err = e.iso.CompileUnboundScript(fmt.Sprintf("faker.%s()", method), "faker-exec", v8go.CompileOptions{})
		if err != nil {
			return "", err
		}

		e.scripts[method] = us
	}

	v, err := us.Run(e.vm)
	if err != nil {
		return "", err
	}

	return v.String(), nil
}

func (e *v8Precompiled) GenerateN(method string, arg any, n int) ([]string, error) {
	out := make([]string, 0, n)
	for range n {
		s, err := e.Generate(method, arg)
		if err != nil {
			return nil, err
		}

		out = append(out, s)
	}

	return out, nil
}

func init() {
	engines = append(engines,
		engineFactory{"v8go-script-per-call", newV8},
		engineFactory{"v8go-precompiled", newV8Precompiled},
	)
}
