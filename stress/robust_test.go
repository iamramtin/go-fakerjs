package stress

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iamramtin/go-fakerjs"
	"github.com/iamramtin/go-fakerjs/conformance"
)

var factory = conformance.Factory{
	Name:   "stress-en",
	Strict: true,
	New: func(seed int64, ref time.Time) (conformance.Engine, error) {
		return fakerjs.NewEngine(seed, fakerjs.WithRefDate(ref)), nil
	},
}

func settle() {
	runtime.GC()
	debug.FreeOSMemory()
}

// TestGCStress is test C1: with a tiny GC target, a goroutine forcing collections and returning memory to
// the OS in a loop, 200k+ golden calls must still reproduce the golden outputs. Run under GOGC=1 and once
// under GODEBUG=gccheckmark=1 by `make test-stress`.
func TestGCStress(t *testing.T) {
	old := debug.SetGCPercent(1)
	defer debug.SetGCPercent(old)

	stop := make(chan struct{})

	var (
		gcs atomic.Int64
		wg  sync.WaitGroup
	)

	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				runtime.GC()
				debug.FreeOSMemory()
				gcs.Add(1)
			}
		}
	})

	outs := conformance.Evaluate(t, factory, conformance.Workload22)
	outs = append(outs, conformance.Evaluate(t, factory, conformance.Interleaved)...)

	close(stop)
	wg.Wait()

	calls := 0
	for _, o := range outs {
		calls += len(o.Case.R)
		if o.Disposition != conformance.Exact {
			t.Errorf("%s diverged under GC pressure at call #%d: want %s got %s", o.Case.ID, o.Index, o.Expected, o.Got)
		}
	}

	t.Logf("%d calls verified against golden across %d forced GCs", calls, gcs.Load())

	if calls < 200_000 {
		t.Fatalf("only %d calls", calls)
	}
}

// TestLeak is test C2: a million calls on one engine; RSS after warm-up vs the end within +10%, and no
// goroutines left behind.
func TestLeak(t *testing.T) {
	calls := 1_000_000
	if testing.Short() {
		calls = 100_000
	}

	methods := []struct {
		m string
		a any
	}{
		{"person.firstName", nil},
		{"internet.email", nil},
		{"lorem.sentence", nil},
		{"location.streetAddress", nil},
		{"string.alpha", map[string]any{"length": 20}},
		{"commerce.productDescription", nil},
		{"date.recent", nil},
		{"helpers.arrayElements", []string{"a", "b", "c", "d"}},
		{"company.name", nil},
		{"string.uuid", nil},
	}

	e := fakerjs.NewEngine(42, fakerjs.WithRefDate(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	defer e.Close()

	goroutines := runtime.NumGoroutine()

	var warm uint64

	for i := 0; i < calls; i++ {
		m := methods[i%len(methods)]
		if _, err := e.Generate(m.m, m.a); err != nil {
			t.Fatal(err)
		}

		if i == calls/10 {
			settle()

			warm = RSS()
		}
	}

	settle()

	end := RSS()
	t.Logf("RSS after warm-up %d MB, at end %d MB", warm>>20, end>>20)

	if warm == 0 {
		t.Skip("RSS unavailable on this platform")
	}

	if float64(end) > float64(warm)*1.10 {
		t.Errorf("RSS grew from %d MB to %d MB (> +10%%)", warm>>20, end>>20)
	}

	if g := runtime.NumGoroutine(); g != goroutines {
		t.Errorf("goroutines: %d before, %d after", goroutines, g)
	}
}

// TestInstanceChurn is test C3: creating and closing thousands of engines returns memory to the baseline.
func TestInstanceChurn(t *testing.T) {
	n := 2000
	if testing.Short() {
		n = 200
	}

	for i := range 10 { // warm the allocator
		e := fakerjs.NewEngine(int64(i + 1))
		_, _ = e.Generate("person.firstName", nil)
		e.Close()
	}

	settle()

	base := RSS()
	goroutines := runtime.NumGoroutine()

	for i := 0; i < n; i++ {
		e := fakerjs.NewEngine(int64(i + 1))
		if _, err := e.Generate("person.firstName", nil); err != nil {
			t.Fatal(err)
		}

		if i%2 == 0 {
			e.Close() // odd ones are simply dropped: memory is plain Go heap, no finalizer needed
		}
	}

	settle()

	end := RSS()
	t.Logf("baseline %d MB, after %d engines %d MB", base>>20, n, end>>20)

	if base == 0 {
		t.Skip("RSS unavailable on this platform")
	}

	if end > base+64<<20 {
		t.Errorf("RSS %d MB after churn vs baseline %d MB: module memory leaked", end>>20, base>>20)
	}

	if g := runtime.NumGoroutine(); g != goroutines {
		t.Errorf("goroutines: %d before, %d after", goroutines, g)
	}
}

// TestIsolation is test C4: interleaving two engines never lets one observe the other.
func TestIsolation(t *testing.T) {
	cases, err := conformance.Load(conformance.Workload22)
	if err != nil {
		t.Fatal(err)
	}

	find := func(id string) *conformance.Case {
		for i := range cases {
			if cases[i].ID == id {
				return &cases[i]
			}
		}

		t.Fatalf("golden case %s missing", id)

		return nil
	}

	pairs := [][2]string{
		{"e22:person.firstName@1", "e22:person.firstName@42"},
		{"e22:internet.email@1", "e22:lorem.sentence@42"},
		{"e22:location.city@100", "e22:string.alpha@100"},
	}
	for _, p := range pairs {
		ca, cb := find(p[0]), find(p[1])
		mk := func(c *conformance.Case) *fakerjs.Engine {
			var seed int64
			if _, err := fmt.Sscan(c.Seed, &seed); err != nil {
				t.Fatalf("%s: seed %q: %v", c.ID, c.Seed, err)
			}

			return fakerjs.NewEngine(seed, fakerjs.WithRefDate(conformance.RefDate))
		}

		a, b := mk(ca), mk(cb)
		for i := 0; i < len(ca.R); i++ {
			ga, err := a.Generate(ca.Calls[0].M, ca.Calls[0].A)
			if err != nil || ga != ca.R[i].Value {
				t.Fatalf("%s call %d: got (%q,%v) want %q", p[0], i, ga, err, ca.R[i].Value)
			}

			gb, err := b.Generate(cb.Calls[0].M, cb.Calls[0].A)
			if err != nil || gb != cb.R[i].Value {
				t.Fatalf("%s call %d: got (%q,%v) want %q", p[1], i, gb, err, cb.R[i].Value)
			}
		}

		a.Close()
		b.Close()
	}
}
