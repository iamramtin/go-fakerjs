package stress

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/iamramtin/go-fakerjs"
	"github.com/iamramtin/go-fakerjs/conformance"
)

// fuzzEngine is shared by iterations of one fuzz worker for speed. It is replaced when poisoned or after
// many iterations so one bad state cannot hide behind another.
var (
	fuzzMu    sync.Mutex
	fuzzEng   *fakerjs.Engine
	fuzzCalls int
)

func fuzzEngine() *fakerjs.Engine {
	fuzzMu.Lock()
	defer fuzzMu.Unlock()

	if fuzzEng == nil || fuzzEng.Poisoned() || fuzzCalls > 2000 {
		if fuzzEng != nil {
			fuzzEng.Close()
		}

		fuzzEng = fakerjs.NewEngine(1, fakerjs.WithTimeout(2*time.Second), fakerjs.WithRefDate(conformance.RefDate))
		fuzzCalls = 0
	}

	fuzzCalls++

	return fuzzEng
}

// FuzzGenerate is test C7: arbitrary (method, JSON argument) pairs must never panic, never hang past a 5 s
// watchdog, and must produce valid UTF-8 or an error.
func FuzzGenerate(f *testing.F) {
	cases, err := conformance.Load(conformance.Errors)
	if err != nil {
		f.Fatal(err)
	}

	for _, c := range cases {
		a, _ := json.Marshal(c.Calls[0].A)
		f.Add(c.Calls[0].M, a)
	}

	pm, err := conformance.Load(conformance.Coercion)
	if err != nil {
		f.Fatal(err)
	}

	for _, c := range pm {
		a, _ := json.Marshal(c.Calls[0].A)
		f.Add(c.Calls[0].M, a)
	}

	for _, m := range []string{"person.constructor", "person.toString", "person.hasOwnProperty", "person.valueOf", "person.__proto__", "person.prototype", "helpers.constructor", "person.faker"} {
		f.Add(m, []byte("null"))
	}

	f.Add("string.alpha", []byte(`{"length":{"min":2,"max":4}}`))
	f.Add("helpers.fake", []byte(`"{{person.firstName}} {{helpers.fake(\"{{string.alpha}}\")}}"`))
	f.Add("lorem.words", []byte(`5`))
	f.Add("helpers.fromRegExp", []byte(`"[a-z]{3}\\d+"`))
	f.Add("string.alpha", []byte(`{"length":1073741824}`))

	f.Fuzz(func(t *testing.T, method string, argJSON []byte) {
		var arg any
		if len(argJSON) > 0 {
			arg = json.RawMessage(argJSON) // json.Marshal validates it; invalid JSON is a plain error
		}

		type result struct {
			v   string
			err error
		}

		done := make(chan result, 1)

		go func() {
			v, err := fuzzEngine().Generate(method, arg)
			done <- result{v, err}
		}()

		select {
		case r := <-done:
			if r.err == nil && !utf8.ValidString(r.v) {
				t.Fatalf("invalid UTF-8 result for %q: %q", method, r.v)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("hang: Generate(%q, %q) still running after 5 s", method, argJSON)
		}
	})
}
