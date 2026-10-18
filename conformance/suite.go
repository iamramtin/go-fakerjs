package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

// Evaluate runs every case of one golden kind and returns the outcomes in file order.
// Cases are independent (each builds its own engine), so they run on GOMAXPROCS workers.
func Evaluate(tb testing.TB, f Factory, kind string) []Outcome {
	tb.Helper()

	cases, err := Load(kind)
	if err != nil {
		tb.Fatalf("load %s: %v", kind, err)
	}

	out := make([]Outcome, len(cases))
	errs := make([]error, len(cases))

	var wg sync.WaitGroup

	next := make(chan int)

	for w := 0; w < runtime.GOMAXPROCS(0); w++ {
		wg.Go(func() {
			for i := range next {
				got, err := Run(f, &cases[i])
				if err != nil {
					errs[i] = err
					continue
				}

				out[i] = Judge(&cases[i], got)
			}
		})
	}

	for i := range cases {
		next <- i
	}

	close(next)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			tb.Fatalf("case %s: %v", cases[i].ID, err)
		}
	}

	return out
}

func record(tb testing.TB, f Factory, outs []Outcome) {
	tb.Helper()

	r := NewReport(f.Name)
	for _, o := range outs {
		r.Add(o)
	}

	if err := r.Write(); err != nil {
		tb.Logf("write report: %v", err)
	}

	cats := map[string]*Tally{}
	for _, o := range outs {
		t := cats[o.Case.Kind]
		if t == nil {
			t = &Tally{}
			cats[o.Case.Kind] = t
		}

		t.add(o.Disposition)
	}

	for k, t := range cats {
		tb.Logf("[%s] %-12s cases=%d exact=%d normalized=%d divergent=%d", f.Name, k, t.Total(), t.Exact, t.Normalized, t.Divergent)
	}
}

func failDivergent(t *testing.T, f Factory, o Outcome) {
	t.Helper()
	t.Errorf("%s: DIVERGENT at call #%d (seed %s)\n  expected %s\n  got      %s\n  context:\n    %s",
		o.Case.ID, o.Index, o.Case.Seed, o.Expected, o.Got, strings.Join(o.Calls, "\n    "))
}

// Workload22Exact is test A1: every case on the 22 workload paths must match byte for
// byte. NORMALIZED does not count; no divergence may be allowlisted.
func Workload22Exact(t *testing.T, f Factory) {
	outs := Evaluate(t, f, Workload22)
	record(t, f, outs)

	nNorm := 0

	for _, o := range outs {
		switch o.Disposition {
		case Exact:
		case Divergent:
			failDivergent(t, f, o)
		case Normalized:
			nNorm++

			t.Errorf("%s: only NORMALIZED, byte-exact required for workload paths", o.Case.ID)
		}
	}

	if nNorm > 0 {
		t.Logf("%d workload-22 cases pass only after normalization", nNorm)
	}
}

// InterleavedExact is test A2: the three 5,000-call mixed sequences must match for their entire length.
func InterleavedExact(t *testing.T, f Factory) {
	outs := Evaluate(t, f, Interleaved)
	record(t, f, outs)

	for _, o := range outs {
		switch o.Disposition {
		case Exact:
		case Divergent:
			failDivergent(t, f, o)
		case Normalized:
			t.Errorf("%s: only NORMALIZED, byte-exact required for interleaved sequences", o.Case.ID)
		}
	}
}

// PerMethodSeeded is test A3. A DIVERGENT case fails unless it is listed in known_divergences.yaml, and
// a listed case that now passes is a stale entry and also fails. Non-strict factories only report.
func PerMethodSeeded(t *testing.T, f Factory) {
	outs := Evaluate(t, f, PerMethod)
	record(t, f, outs)

	known, err := LoadDivergences()
	if err != nil {
		t.Fatal(err)
	}

	used := make([]bool, len(known))

	for _, o := range outs {
		listed := -1

		for i, d := range known {
			if d.Matches(o.Case) {
				listed, used[i] = i, true
			}
		}

		switch {
		case o.Disposition == Divergent && listed < 0:
			if f.Strict {
				failDivergent(t, f, o)
			}
		case o.Disposition != Divergent && listed >= 0 && f.Strict:
			// An entry may be wildcarded over several cases of which only some diverge, so only an entry
			// whose every matching case passes is stale; checked below.
		}
	}

	if !f.Strict {
		return
	}

	for i, d := range known {
		if !used[i] {
			t.Errorf("stale known_divergences entry (matches no case): %+v", d)
			continue
		}

		stillDiverges := false

		for _, o := range outs {
			if d.Matches(o.Case) && o.Disposition == Divergent {
				stillDiverges = true
				break
			}
		}

		if !stillDiverges {
			t.Errorf("stale known_divergences entry (all matching cases now pass): %+v", d)
		}
	}
}

// reportOrFail is shared by the smaller categories: strict factories fail on DIVERGENT, others report.
func reportOrFail(t *testing.T, f Factory, kind string) {
	outs := Evaluate(t, f, kind)
	record(t, f, outs)
	sort.SliceStable(outs, func(i, j int) bool { return outs[i].Case.ID < outs[j].Case.ID })

	for _, o := range outs {
		if o.Disposition == Divergent {
			if f.Strict {
				failDivergent(t, f, o)
			} else {
				t.Logf("(non-strict) %s diverges: %s", o.Case.ID, o.describe())
			}
		}
	}
}

// SeedEdges is test A4: seeds whose float64 conversion loses precision, negatives and MaxInt64.
func SeedEdges(t *testing.T, f Factory) { reportOrFail(t, f, SeedEdge) }

// CoercionMatchesJSString is test A5.
func CoercionMatchesJSString(t *testing.T, f Factory) { reportOrFail(t, f, Coercion) }

// ErrorsMatchClass is test A6: error-vs-success parity plus error-class parity.
func ErrorsMatchClass(t *testing.T, f Factory) { reportOrFail(t, f, Errors) }

// DumpHash runs every golden case on f and returns a sha256 over the engine's own output (not the
// expectations). Two builds, variants or architectures are interchangeable on the golden set exactly when
// their dumps are equal.
func DumpHash(tb testing.TB, f Factory) string {
	tb.Helper()

	h := sha256.New()

	for _, kind := range []string{PerMethod, SeedEdge, Interleaved, Workload22, Coercion, Errors} {
		cases, err := Load(kind)
		if err != nil {
			tb.Fatal(err)
		}

		res := make([][]Result, len(cases))

		var wg sync.WaitGroup

		sem := make(chan struct{}, runtime.GOMAXPROCS(0))

		for i := range cases {
			wg.Add(1)

			sem <- struct{}{}

			go func() {
				defer wg.Done()
				defer func() { <-sem }()

				r, err := Run(f, &cases[i])
				if err != nil {
					tb.Errorf("case %s: %v", cases[i].ID, err)
				}

				res[i] = r
			}()
		}

		wg.Wait()

		for i, rs := range res {
			_, _ = fmt.Fprintf(h, "%s|%d\n", cases[i].ID, len(rs))

			for _, r := range rs {
				_, _ = fmt.Fprintf(h, "%d:%s:%s\n", len(r.Value), r.Err, r.Value)
			}
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}
