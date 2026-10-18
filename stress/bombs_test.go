package stress

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iamramtin/go-fakerjs"
	"github.com/iamramtin/go-fakerjs/conformance"
)

type bomb struct {
	name   string
	method string
	arg    any
	n      int
	opts   []fakerjs.Option
}

func nested(depth int) any {
	var v any = 1
	for range depth {
		v = map[string]any{"a": v}
	}

	return v
}

// nestedTemplate nests depth levels of braces: {{{{...person.firstName...}}}}.
func nestedTemplate(depth int) string {
	return strings.Repeat("{{", depth) + "person.firstName" + strings.Repeat("}}", depth)
}

// escapedTemplate nests helpers.fake calls through JSON string arguments. Every level re-escapes the inner
// template, so its size doubles per level; depth is therefore kept small.
func escapedTemplate(depth int) string {
	t := "{{person.firstName}}"
	for range depth {
		t = "{{helpers.fake(\"" + strings.ReplaceAll(strings.ReplaceAll(t, `\`, `\\`), `"`, `\"`) + "\")}}"
	}

	return t
}

// bombs are calls whose arguments would exhaust memory or time if the engine let them.
func bombs() []bomb {
	return []bomb{
		{name: "string.alpha length 2^30", method: "string.alpha", arg: map[string]any{"length": 1 << 30}},
		{name: "string.alpha length 2^24", method: "string.alpha", arg: map[string]any{"length": 1 << 24}},
		{name: "string.numeric length 1e9", method: "string.numeric", arg: map[string]any{"length": 1e9}},
		{name: "lorem.paragraphs 1e8", method: "lorem.paragraphs", arg: 1e8},
		{name: "lorem.words 1e9", method: "lorem.words", arg: 1e9},
		{name: "lorem.text via GenerateN 1e8", method: "lorem.text", n: 1e8},
		{name: "person.firstName GenerateN 1e9", method: "person.firstName", n: 1e9},
		{name: "helpers.fromRegExp a{1e8}", method: "helpers.fromRegExp", arg: "a{100000000}"},
		{name: "helpers.fromRegExp nested quantifiers", method: "helpers.fromRegExp", arg: "((a{1000}){1000}){1000}"},
		{name: "helpers.arrayElements giant count", method: "helpers.arrayElements", arg: make([]int, 1<<20)},
		{name: "helpers.shuffle 2^20 elements", method: "helpers.shuffle", arg: make([]int, 1<<20)},
		{name: "helpers.slugify 3 MiB", method: "helpers.slugify", arg: strings.Repeat("a b ", 700000)},
		{name: "nested options JSON depth 10000", method: "string.alpha", arg: nested(10000)},
		{name: "nested options JSON depth 100000", method: "string.alpha", arg: nested(100000), opts: []fakerjs.Option{fakerjs.WithMaxArgBytes(64 << 20)}},
		{name: "50 MB args (default cap)", method: "helpers.slugify", arg: strings.Repeat("x", 50<<20)},
		{name: "50 MB args (cap lifted)", method: "helpers.slugify", arg: strings.Repeat("x", 50<<20), opts: []fakerjs.Option{fakerjs.WithMaxArgBytes(128 << 20)}},
		{name: "50 MB method string", method: strings.Repeat("a", 50<<20) + ".b"},
		{name: "helpers.fake escaped nesting depth 14", method: "helpers.fake", arg: escapedTemplate(14)},
		{name: "helpers.fake braces nested 500", method: "helpers.fake", arg: nestedTemplate(500)},
		{name: "helpers.fake braces nested 100000", method: "helpers.fake", arg: nestedTemplate(100000)},
		{name: "helpers.fake template 3 MiB", method: "helpers.fake", arg: strings.Repeat("{{person.firstName}}", 150000)},
		{name: "string.nanoid runaway loop", method: "string.nanoid", arg: map[string]any{"length": 5}},
	}
}

// TestResourceBombs is test C5. Every bomb must end in a Go error (or a success) within a bounded time and
// bounded memory, without crashing the process; afterwards the engine is either usable or reports
// ErrPoisoned. Observed limits are written to reports/bombs.json.
func TestResourceBombs(t *testing.T) {
	const wall = 20 * time.Second * raceFactor

	type row struct {
		Name       string `json:"name"`
		Outcome    string `json:"outcome"`
		DurationMS int64  `json:"durationMs"`
		PeakRSSMB  uint64 `json:"peakRssMB"`
		Usable     bool   `json:"engineUsableAfter"`
	}

	var rows []row

	for _, b := range bombs() {
		t.Run(b.name, func(t *testing.T) {
			opts := append([]fakerjs.Option{fakerjs.WithTimeout(5 * time.Second)}, b.opts...)

			e := fakerjs.NewEngine(42, opts...)
			defer e.Close()

			before := PeakRSS()
			t0 := time.Now()
			done := make(chan error, 1)

			go func() {
				n := b.n
				if n == 0 {
					n = 1
				}

				_, err := e.GenerateN(b.method, b.arg, n)
				done <- err
			}()

			var err error
			select {
			case err = <-done:
			case <-time.After(wall):
				t.Fatalf("call still running after %v: unbounded", wall)
			}

			dur := time.Since(t0)
			outcome := "ok"

			var fe *fakerjs.Error
			switch {
			case err == nil:
			case errors.As(err, &fe):
				outcome = fe.Class
				if fe.Err != nil {
					outcome += ":" + fe.Err.Error()
				}
			default:
				outcome = "error:" + err.Error()
			}

			usable := false

			if !e.Poisoned() {
				_, uerr := e.Generate("person.firstName", nil)
				usable = uerr == nil
			}

			peak := PeakRSS()
			rows = append(rows, row{b.name, outcome, dur.Milliseconds(), peak >> 20, usable})
			t.Logf("outcome=%s duration=%v peakRSS=%dMB (+%dMB) usable=%v", outcome, dur.Round(time.Millisecond), peak>>20, (peak-before)>>20, usable)

			if !usable && !e.Poisoned() {
				t.Errorf("engine neither usable nor poisoned after %s", b.name)
			}

			if dur > 12*time.Second*raceFactor {
				t.Errorf("took %v; want a bounded response", dur)
			}

			if peak > raceFactor*1500<<20 {
				t.Errorf("peak RSS %d MB exceeds the 1.5 GB budget", peak>>20)
			}
		})
	}

	out, _ := json.MarshalIndent(rows, "", " ")
	_ = os.WriteFile(filepath.Join(conformance.ReportsDir(), "bombs.json"), out, 0o644)
}

// TestPoolRecoversFromBomb: a poisoned engine is replaced on release and the pool keeps working.
func TestPoolRecoversFromBomb(t *testing.T) {
	p := fakerjs.NewPool(2, 5, fakerjs.WithTimeout(2*time.Second))
	defer p.Close()

	for i := range 4 {
		_ = p.Do(func(e *fakerjs.Engine) error {
			_, err := e.Generate("string.alpha", map[string]any{"length": 1 << 30})
			return err
		})
		if err := p.Do(func(e *fakerjs.Engine) error { _, err := e.Generate("person.firstName", nil); return err }); err != nil {
			t.Fatalf("round %d: pool unusable after bomb: %v", i, err)
		}
	}
}

// TestDeadline is test C6: a runaway call returns ErrDeadline within 2x the requested timeout.
func TestDeadline(t *testing.T) {
	for _, d := range []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second} {
		e := fakerjs.NewEngine(42, fakerjs.WithTimeout(d))
		if _, err := e.Generate("person.firstName", nil); err != nil {
			t.Fatal(err)
		}

		t0 := time.Now()
		_, err := e.Generate("string.nanoid", map[string]any{"length": 5}) // loops forever in Faker.js 10.6.0 itself
		took := time.Since(t0)

		if !errors.Is(err, fakerjs.ErrDeadline) {
			t.Fatalf("timeout %v: err = %v, want ErrDeadline", d, err)
		}

		if took > 2*d {
			t.Errorf("timeout %v: returned after %v (> 2x)", d, took)
		}

		t.Logf("timeout %v -> ErrDeadline after %v", d, took.Round(time.Millisecond))
		// The interrupted engine remains usable and deterministic-per-call.
		if v, err := e.Generate("person.firstName", nil); err != nil || v == "" {
			t.Errorf("engine unusable after deadline: (%q, %v)", v, err)
		}

		if e.Poisoned() {
			t.Error("deadline should not poison the engine")
		}

		e.Close()
	}
	// No deadline set: a short call is unaffected.
	e := fakerjs.NewEngine(1, fakerjs.WithTimeout(0))
	defer e.Close()

	if _, err := e.Generate("person.firstName", nil); err != nil {
		t.Fatal(err)
	}

	fmt.Print()
}
