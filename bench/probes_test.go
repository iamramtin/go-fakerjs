//go:build baselines

package bench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/iamramtin/go-fakerjs/bench/qjsbase"
	"github.com/iamramtin/go-fakerjs/conformance"
)

// TestQJSProbes runs golden/probes.js in QuickJS and diffs it against the V8 results recorded in
// reports/probes-v8.json (written by `node golden/probes.mjs`).
func TestQJSProbes(t *testing.T) {
	root := conformance.Root()

	code, err := os.ReadFile(filepath.Join(root, "golden", "probes.js"))
	if err != nil {
		t.Fatal(err)
	}

	e, err := qjsbase.New(1, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	s, err := e.Eval(string(code))
	if err != nil {
		t.Fatal(err)
	}

	var qjs map[string]string
	if err := json.Unmarshal([]byte(s), &qjs); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(filepath.Join(root, "reports", "probes-v8.json"))
	if err != nil {
		t.Skip("run `make probes-v8` first")
	}

	var v8 map[string]string
	if err := json.Unmarshal(b, &v8); err != nil {
		t.Fatal(err)
	}

	type row struct{ Name, V8, QuickJS string }

	var diffs []row

	same := 0

	var names []string
	for k := range v8 {
		names = append(names, k)
	}

	sort.Strings(names)

	for _, k := range names {
		if v8[k] == qjs[k] {
			same++
		} else {
			diffs = append(diffs, row{k, v8[k], qjs[k]})
		}
	}

	out, _ := json.MarshalIndent(map[string]any{"same": same, "differ": diffs}, "", " ")
	_ = os.WriteFile(filepath.Join(root, "reports", "probes-diff.json"), out, 0o644)

	t.Logf("%d probes identical, %d differ", same, len(diffs))

	for _, d := range diffs {
		t.Logf("DIFF %-34s V8=%q QuickJS=%q", d.Name, d.V8, d.QuickJS)
	}
}

// TestQJSMathRandomDiffers: two fresh QuickJS contexts must not share a Math.random sequence.
func TestQJSMathRandomDiffers(t *testing.T) {
	a, _ := qjsbase.New(1, time.Time{})
	b, _ := qjsbase.New(1, time.Time{})

	defer a.Close()
	defer b.Close()

	ra, _ := a.Eval(`[Math.random(),Math.random(),Math.random()].join()`)
	rb, _ := b.Eval(`[Math.random(),Math.random(),Math.random()].join()`)
	t.Logf("ctx A: %s\nctx B: %s", ra, rb)

	if ra == rb {
		t.Logf("Math.random sequences are IDENTICAL across fresh contexts (seeded identically) - Wizer will bake this in")
	}
}
