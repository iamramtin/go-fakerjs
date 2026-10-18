// Package contract_test checks the engine contract: the behaviour of the V8-based adapter this library
// would replace. Inputs and expectations mirror that adapter's own tests; no code is shared.
package contract_test

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/iamramtin/go-fakerjs"
)

var refDate = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func mustGen(t *testing.T, e *fakerjs.Engine, method string, arg any) string {
	t.Helper()

	v, err := e.Generate(method, arg)
	if err != nil {
		t.Fatalf("Generate(%q, %v): %v", method, arg, err)
	}

	return v
}

func seq(t *testing.T, e *fakerjs.Engine, method string, arg any, n int) []string {
	t.Helper()

	out := make([]string, n)
	for i := range out {
		out[i] = mustGen(t, e, method, arg)
	}

	return out
}

// B1
func TestValidMethodTable(t *testing.T) {
	valid := []string{"person.firstName", "internet.ipv4", "a.b", "A1.b2", "location.zipCode"}
	invalid := []string{
		"", "person", "person.", ".x", "1a.b", "a.b.c", "a.b()", "a.b;c", "person.firstName();globalThis.x=1",
		"a-b.c", "a b.c", "a.b\n", "a.b\r\n", "\na.b", "a._b", "_a.b", "a.b ", " a.b", "a..b", "ä.b", "a.bé", "a.b\x00",
		"nodot", "too.many.dots", "person.first-name", "1nternet.ipv4", "person.name();alert(1)",
	}

	for _, m := range valid {
		if !fakerjs.ValidMethod(m) {
			t.Errorf("ValidMethod(%q) = false, want true", m)
		}
	}

	for _, m := range invalid {
		if fakerjs.ValidMethod(m) {
			t.Errorf("ValidMethod(%q) = true, want false", m)
		}
	}
}

// B1, cross-checked against the regexp the V8 adapter uses, over all strings of a small alphabet.
func TestValidMethodMatchesRegexp(t *testing.T) {
	re := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*\.[a-zA-Z][a-zA-Z0-9]*$`)
	alphabet := []string{"a", "Z", "0", ".", "_", "\n", "é"}

	var rec func(prefix string, depth int)

	rec = func(prefix string, depth int) {
		if got, want := fakerjs.ValidMethod(prefix), re.MatchString(prefix); got != want {
			t.Fatalf("ValidMethod(%q) = %v, regexp says %v", prefix, got, want)
		}

		if depth == 0 {
			return
		}

		for _, a := range alphabet {
			rec(prefix+a, depth-1)
		}
	}
	rec("", 6)
}

// B2
func TestEmptyAndInvalidMethodErrors(t *testing.T) {
	e := fakerjs.NewEngine(0)
	defer e.Close()

	if _, err := e.Generate("", nil); err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("empty method: err = %v, want 'cannot be empty'", err)
	}

	for _, m := range []string{"nodot", "too.many.dots", "person.first-name", "1nternet.ipv4", "person.name();alert(1)", "a.b\n"} {
		_, err := e.Generate(m, nil)
		if err == nil || !strings.Contains(err.Error(), "invalid faker method format") {
			t.Errorf("Generate(%q): err = %v, want 'invalid faker method format'", m, err)
		}

		if !fakerjs.IsClass(err, fakerjs.ClassInvalidMethod) {
			t.Errorf("Generate(%q): class != invalid-method", m)
		}
	}
}

// B3
func TestUnknownMethodLeavesEngineUsable(t *testing.T) {
	control := fakerjs.NewEngine(42)
	defer control.Close()

	want := seq(t, control, "person.firstName", nil, 6)

	e := fakerjs.NewEngine(42)
	defer e.Close()

	var got []string
	for _, bad := range []string{"unknown.method", "person.nomethod", "nomodule.firstName", "unknown.method"} {
		got = append(got, mustGen(t, e, "person.firstName", nil))

		_, err := e.Generate(bad, nil)
		if err == nil {
			t.Fatalf("Generate(%q) succeeded", bad)
		}

		if !fakerjs.IsClass(err, fakerjs.ClassUnknownMethod) || !errors.Is(err, fakerjs.ErrUnknownMethod) {
			t.Errorf("Generate(%q): %v is not an unknown-method error", bad, err)
		}
	}

	got = append(got, seq(t, e, "person.firstName", nil, 2)...)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("failed calls perturbed the sequence:\n got %v\nwant %v", got, want)
	}
}

// B4
func TestNilArgVersusOptionsArg(t *testing.T) {
	e := fakerjs.NewEngine(1)
	defer e.Close()

	cases := []struct {
		name     string
		arg      any
		min, max int
	}{
		{"nil", nil, 1, 1},
		{"number", 5, 5, 5},
		{"options", map[string]any{"length": 5}, 5, 5},
		{"range", map[string]any{"length": map[string]any{"min": 2, "max": 4}}, 2, 4},
	}
	for _, c := range cases {
		for range 50 {
			got := mustGen(t, e, "string.alpha", c.arg)
			if n := len(got); n < c.min || n > c.max {
				t.Fatalf("%s: len(%q) = %d, want in [%d,%d]", c.name, got, n, c.min, c.max)
			}
		}
	}
	// "an argument reaches the JavaScript call, not be dropped"
	if got := mustGen(t, e, "string.alpha", map[string]any{"length": 12}); len(got) != 12 {
		t.Fatalf("length arg dropped: %q", got)
	}
	// Digits in method names are accepted.
	if got := mustGen(t, e, "internet.ipv4", nil); !regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}$`).MatchString(got) {
		t.Fatalf("internet.ipv4 = %q", got)
	}
}

// B5
func TestSeedSemantics(t *testing.T) {
	a, b := fakerjs.NewEngine(42), fakerjs.NewEngine(42)
	defer a.Close()
	defer b.Close()

	sa := append(seq(t, a, "person.firstName", nil, 20), seq(t, a, "string.uuid", nil, 20)...)

	sb := append(seq(t, b, "person.firstName", nil, 20), seq(t, b, "string.uuid", nil, 20)...)
	if fmt.Sprint(sa) != fmt.Sprint(sb) {
		t.Fatal("same nonzero seed must give identical sequences across engines")
	}

	c := fakerjs.NewEngine(7)
	defer c.Close()

	if fmt.Sprint(seq(t, c, "person.firstName", nil, 20)) == fmt.Sprint(sa[:20]) {
		t.Fatal("different seeds must diverge")
	}

	// seed 0: Faker's own non-deterministic default. Two fresh engines must differ, which also proves the
	// Math.random state Wizer baked into the snapshot was reseeded per instance.
	for round := range 5 {
		z1, z2 := fakerjs.NewEngine(0), fakerjs.NewEngine(0)
		v1 := append(seq(t, z1, "person.firstName", nil, 20), seq(t, z1, "string.uuid", nil, 20)...)
		v2 := append(seq(t, z2, "person.firstName", nil, 20), seq(t, z2, "string.uuid", nil, 20)...)

		z1.Close()
		z2.Close()

		if fmt.Sprint(v1) == fmt.Sprint(v2) {
			t.Fatalf("round %d: two seed-0 engines produced identical output", round)
		}
	}

	// One seed-0 engine never repeats itself.
	z := fakerjs.NewEngine(0)
	defer z.Close()

	first := seq(t, z, "string.uuid", nil, 50)
	second := seq(t, z, "string.uuid", nil, 50)

	seen := map[string]bool{}
	for _, v := range append(first, second...) {
		if seen[v] {
			t.Fatalf("seed-0 engine repeated %q", v)
		}

		seen[v] = true
	}
}

// Contract: a differently seeded engine used in between must not perturb another engine's sequence.
func TestSeededEngineIsIsolated(t *testing.T) {
	a := fakerjs.NewEngine(42)
	defer a.Close()

	first := mustGen(t, a, "person.firstName", nil)

	b := fakerjs.NewEngine(99)
	defer b.Close()

	mustGen(t, b, "person.firstName", nil)
	second := mustGen(t, a, "person.firstName", nil)

	ref := fakerjs.NewEngine(42)
	defer ref.Close()

	if g := mustGen(t, ref, "person.firstName", nil); g != first {
		t.Fatalf("first: %q != %q", g, first)
	}

	if g := mustGen(t, ref, "person.firstName", nil); g != second {
		t.Fatalf("second: %q != %q (the other engine leaked into this one)", g, second)
	}
}

// B6
func TestLifecycle(t *testing.T) {
	t.Run("lazy init", func(t *testing.T) {
		allocs := testing.AllocsPerRun(100, func() { fakerjs.NewEngine(1).Close() })
		if allocs > 4 {
			t.Fatalf("NewEngine+Close made %v allocations; construction must not build the instance", allocs)
		}

		t0 := time.Now()
		e := fakerjs.NewEngine(1)
		construct := time.Since(t0)
		t0 = time.Now()

		mustGen(t, e, "person.firstName", nil)

		first := time.Since(t0)

		e.Close()

		if construct > time.Millisecond {
			t.Fatalf("NewEngine took %v, want a cheap constructor", construct)
		}

		t.Logf("NewEngine %v, first Generate %v", construct, first)
	})
	t.Run("close before first use", func(t *testing.T) {
		e := fakerjs.NewEngine(1)
		e.Close()

		if _, err := e.Generate("person.firstName", nil); !errors.Is(err, fakerjs.ErrClosed) {
			t.Fatalf("err = %v, want ErrClosed", err)
		}
	})
	t.Run("close twice", func(t *testing.T) {
		e := fakerjs.NewEngine(1)
		mustGen(t, e, "person.firstName", nil)
		e.Close()
		e.Close()
	})
	t.Run("closed engine never restarts its sequence", func(t *testing.T) {
		e := fakerjs.NewEngine(42)
		first := mustGen(t, e, "person.firstName", nil)
		e.Close()

		for range 3 {
			v, err := e.Generate("person.firstName", nil)
			if !errors.Is(err, fakerjs.ErrClosed) || v == first {
				t.Fatalf("after Close: (%q, %v), want ErrClosed", v, err)
			}

			if _, err := e.Generate("string.alpha", map[string]any{"length": 3}); !errors.Is(err, fakerjs.ErrClosed) {
				t.Fatalf("err = %v, want ErrClosed", err)
			}

			if _, err := e.GenerateN("person.firstName", nil, 2); !errors.Is(err, fakerjs.ErrClosed) {
				t.Fatalf("GenerateN err = %v, want ErrClosed", err)
			}
		}
	})
	t.Run("close concurrent with generate", func(t *testing.T) {
		for round := range 20 {
			e := fakerjs.NewEngine(int64(round + 1))

			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() {
					for range 50 {
						_, err := e.Generate("person.firstName", nil)
						if err != nil && !errors.Is(err, fakerjs.ErrClosed) {
							t.Errorf("unexpected error %v", err)
							return
						}
					}
				})
			}

			wg.Go(func() { time.Sleep(time.Duration(round) * 50 * time.Microsecond); e.Close() })

			wg.Wait()
		}
	})
}

// B7
func TestAllowlistBlocksNonMethods(t *testing.T) {
	bad := []string{
		"person.constructor", "person.toString", "person.hasOwnProperty", "person.valueOf", "person.__proto__",
		"person.prototype", "helpers.constructor", "person.faker", "person.isPrototypeOf", "person.toLocaleString",
		"person.propertyIsEnumerable", "faker.seed", "person.definitions", "helpers.fakerCore",
	}

	control := fakerjs.NewEngine(42)
	defer control.Close()

	e := fakerjs.NewEngine(42)
	defer e.Close()

	for _, m := range bad {
		if _, err := e.Generate(m, nil); err == nil {
			t.Errorf("Generate(%q) succeeded, want an error", m)
		}

		if _, err := e.Generate(m, map[string]any{"x": 1}); err == nil {
			t.Errorf("Generate(%q, arg) succeeded, want an error", m)
		}

		if got, want := mustGen(t, e, "person.firstName", nil), mustGen(t, control, "person.firstName", nil); got != want {
			t.Fatalf("after %q the sequence differs from the control: %q != %q", m, got, want)
		}
	}
	// Even valid-looking calls into things that are not Faker methods must not run.
	if _, err := e.Generate("globalThis.eval", "1+1"); err == nil {
		t.Error("globalThis.eval was callable")
	}
}

// B8
func TestRefDateReproducibility(t *testing.T) {
	methods := []struct {
		m string
		a any
	}{
		{"date.recent", nil},
		{"date.past", nil},
		{"date.future", nil},
		{"date.soon", nil},
		{"date.anytime", nil},
		{"date.birthdate", nil},
		{"date.recent", map[string]any{"days": 3}},
		{"date.between", map[string]any{"from": "2020-01-01", "to": "2020-12-31"}},
	}
	run := func(now time.Time, opts ...fakerjs.Option) []string {
		e := fakerjs.NewEngine(42, append([]fakerjs.Option{fakerjs.WithRefDate(refDate), fakerjs.WithClock(func() time.Time { return now })}, opts...)...)
		defer e.Close()

		var out []string

		for range 5 {
			for _, m := range methods {
				out = append(out, mustGen(t, e, m.m, m.a))
			}
		}

		return out
	}
	day1 := run(time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC))

	day2 := run(time.Date(2031, 7, 9, 3, 4, 5, 0, time.UTC))
	if fmt.Sprint(day1) != fmt.Sprint(day2) {
		t.Fatal("with WithRefDate, output must not depend on the host clock")
	}

	// Real clock, two engines 1.1 s apart.
	e1 := fakerjs.NewEngine(42, fakerjs.WithRefDate(refDate))
	a := mustGen(t, e1, "date.recent", nil)

	time.Sleep(1100 * time.Millisecond)

	e2 := fakerjs.NewEngine(42, fakerjs.WithRefDate(refDate))
	b := mustGen(t, e2, "date.recent", nil)

	e1.Close()
	e2.Close()

	if a != b {
		t.Fatalf("engines 1.1 s apart disagree: %q vs %q", a, b)
	}

	// Negative control: without a reference date the faked clock reaches Faker, so the test above is not vacuous.
	noRef := func(now time.Time) string {
		e := fakerjs.NewEngine(42, fakerjs.WithClock(func() time.Time { return now }))
		defer e.Close()

		return mustGen(t, e, "date.recent", nil)
	}
	if noRef(time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)) == noRef(time.Date(2031, 7, 9, 3, 4, 5, 0, time.UTC)) {
		t.Fatal("without WithRefDate the host clock should be visible; the fake clock is not reaching the module")
	}
}

// B9
func TestConcurrentUse(t *testing.T) {
	const goroutines, perG = 64, 2000

	if testing.Short() {
		t.Skip("short")
	}

	single := fakerjs.NewEngine(42)
	want := seq(t, single, "person.firstName", nil, goroutines*perG)
	single.Close()
	sort.Strings(want)

	e := fakerjs.NewEngine(42)
	defer e.Close()

	got := make([][]string, goroutines)

	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			out := make([]string, perG)
			for i := range out {
				v, err := e.Generate("person.firstName", nil)
				if err != nil || v == "" || !utf8.ValidString(v) {
					t.Errorf("goroutine %d call %d: (%q, %v)", g, i, v, err)
					return
				}

				out[i] = v
			}

			got[g] = out
		})
	}

	wg.Wait()

	var all []string
	for _, o := range got {
		all = append(all, o...)
	}

	sort.Strings(all)

	if len(all) != len(want) {
		t.Fatalf("got %d results, want %d", len(all), len(want))
	}

	for i := range all {
		if all[i] != want[i] {
			t.Fatalf("multiset differs from the single-threaded run at sorted index %d: %q != %q", i, all[i], want[i])
		}
	}
}

// GenerateN is n Generate calls in one crossing.
func TestGenerateNEqualsRepeatedGenerate(t *testing.T) {
	for _, c := range []struct {
		m string
		a any
	}{
		{"person.firstName", nil},
		{"string.alpha", map[string]any{"length": 7}},
		{"helpers.shuffle", []any{1, 2, 3, 4, 5, 6, 7, 8}},
		{"helpers.arrayElements", []any{"a", "b", "c", "d", "e"}},
		{"date.recent", nil},
	} {
		for _, n := range []int{0, 1, 2, 10, 100} {
			a := fakerjs.NewEngine(9, fakerjs.WithRefDate(refDate))
			b := fakerjs.NewEngine(9, fakerjs.WithRefDate(refDate))

			got, err := a.GenerateN(c.m, c.a, n)
			if err != nil {
				t.Fatalf("GenerateN(%s, %d): %v", c.m, n, err)
			}

			want := seq(t, b, c.m, c.a, n)
			a.Close()
			b.Close()

			if len(got) != n || fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("GenerateN(%s,%d) != %d x Generate:\n%v\n%v", c.m, n, n, got, want)
			}
		}
	}

	e := fakerjs.NewEngine(1)
	defer e.Close()

	if _, err := e.GenerateN("person.firstName", nil, -1); err == nil {
		t.Fatal("negative n accepted")
	}
}

// Pool seed derivation is documented and deterministic.
func TestPool(t *testing.T) {
	if fakerjs.DeriveSeed(0, 3) != 0 {
		t.Fatal("base 0 must stay non-deterministic")
	}

	if fakerjs.DeriveSeed(42, 0) != 42 {
		t.Fatal("engine 0 uses the base seed")
	}

	seen := map[int64]bool{}

	for i := range 1000 {
		s := fakerjs.DeriveSeed(42, i)
		if s == 0 || seen[s] {
			t.Fatalf("derived seed %d at %d is zero or repeated", s, i)
		}

		seen[s] = true
	}

	p := fakerjs.NewPool(4, 42, fakerjs.WithRefDate(refDate))
	defer p.Close()

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 20 {
				if err := p.Do(func(e *fakerjs.Engine) error { _, err := e.Generate("person.firstName", nil); return err }); err != nil {
					t.Error(err)
				}
			}
		})
	}

	wg.Wait()
	// Engine i of a fresh pool equals a standalone engine seeded DeriveSeed(base, i).
	p2 := fakerjs.NewPool(3, 42)
	defer p2.Close()

	for i := range 3 {
		e, release := p2.Get()
		got := mustGen(t, e, "person.firstName", nil)

		release()

		ref := fakerjs.NewEngine(fakerjs.DeriveSeed(42, i))
		want := mustGen(t, ref, "person.firstName", nil)
		ref.Close()

		if got != want {
			t.Errorf("pool engine %d produced %q, standalone DeriveSeed engine produced %q", i, got, want)
		}
	}
}
