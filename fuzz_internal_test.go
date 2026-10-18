package fakerjs

import (
	"testing"
	"time"
	"unicode/utf8"
)

// FuzzSnapshotArgs feeds raw bytes, valid JSON or not, straight to the module's parser and call path.
func FuzzSnapshotArgs(f *testing.F) {
	for _, s := range []string{"", "null", "1", `"x"`, "[]", "{}", `{"length":5}`, `{"a":`, "\x00", `"\ud800"`, `[1,2,3]`, `{"__proto__":{"x":1}}`} {
		f.Add("string.alpha", []byte(s))
		f.Add("helpers.arrayElement", []byte(s))
		f.Add("helpers.fake", []byte(s))
	}

	var e *Engine

	f.Fuzz(func(t *testing.T, method string, arg []byte) {
		if e == nil || e.Poisoned() {
			e = NewEngine(1, WithTimeout(2*time.Second), WithMaxArgBytes(1<<20))
		}

		if !ValidMethod(method) {
			return
		}

		done := make(chan struct{})
		go func() {
			defer close(done)

			out, err := e.generateJSON(method, arg, 1)
			if err == nil && !utf8.ValidString(out[0]) {
				t.Errorf("invalid UTF-8 from %q", method)
			}
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("hang on %q %q", method, arg)
		}

		if e.Poisoned() {
			t.Fatalf("poisoned by %q %q", method, arg)
		}
	})
}
