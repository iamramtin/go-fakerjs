package conformance_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/iamramtin/go-fakerjs"
	"github.com/iamramtin/go-fakerjs/conformance"
)

func dumpPath(variant string) string {
	return filepath.Join(conformance.ReportsDir(), fmt.Sprintf("golden-dump-%s-%s.sha256", variant, runtime.GOARCH))
}

// TestWriteGoldenDump records the sha256 of the en snapshot's output over the whole golden set, for this
// architecture, in reports/golden-dump-en-<arch>.sha256. CI compares the dumps across
// architectures.
func TestWriteGoldenDump(t *testing.T) {
	sum := conformance.DumpHash(t, nativeFactory)
	if err := os.WriteFile(dumpPath("en"), []byte(sum+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Logf("golden dump en/%s = %s", runtime.GOARCH, sum)
}

// TestBundleFidelity is test A8: the recorded O1 == O2 flag (real npm package vs minified bundle).
func TestBundleFidelity(t *testing.T) {
	meta, err := conformance.LoadMeta()
	if err != nil {
		t.Fatal(err)
	}

	if !meta.O1EqualsO2 {
		t.Fatal("golden/testdata/meta.json records an O1/O2 mismatch: the bundle does not behave like the npm package (see reports/golden-bundle-fidelity.json)")
	}

	api, err := conformance.LoadAPI()
	if err != nil {
		t.Fatal(err)
	}

	if api.FakerVersion != meta.FakerVersion {
		t.Fatalf("api.json is for faker %s but meta.json for %s", api.FakerVersion, meta.FakerVersion)
	}

	pins, err := os.ReadFile(filepath.Join(conformance.Root(), "tools.versions"))
	if err != nil {
		t.Fatal(err)
	}

	if !regexp.MustCompile(`(?m)^faker +@faker-js/faker ` + regexp.QuoteMeta(meta.FakerVersion) + ` `).Match(pins) {
		t.Fatalf("tools.versions does not pin faker %s", meta.FakerVersion)
	}

	if fakerjs.FakerVersion != meta.FakerVersion {
		t.Fatalf("fakerjs.FakerVersion is %s but the golden data is for faker %s", fakerjs.FakerVersion, meta.FakerVersion)
	}
}

// TestAPICoverageComplete is test A9: every enumerated method is exercised by a per-method case or is
// listed with a reason in golden/skipped.yaml.
func TestAPICoverageComplete(t *testing.T) {
	api, err := conformance.LoadAPI()
	if err != nil {
		t.Fatal(err)
	}

	cases, err := conformance.Load(conformance.PerMethod)
	if err != nil {
		t.Fatal(err)
	}

	covered := map[string]bool{}
	for _, c := range cases {
		covered[c.Method()] = true
	}

	skipped, err := conformance.LoadSkipped()
	if err != nil {
		t.Fatal(err)
	}

	skip := map[string]bool{}
	for _, s := range skipped {
		skip[s.Method] = true
	}

	n := 0

	for mod, methods := range api.Modules {
		for _, m := range methods {
			n++

			full := mod + "." + m
			switch {
			case covered[full] && skip[full]:
				t.Errorf("%s is both exercised and listed in skipped.yaml", full)
			case !covered[full] && !skip[full]:
				t.Errorf("%s is enumerated but neither exercised nor in skipped.yaml", full)
			}
		}
	}

	if n != api.Total {
		t.Errorf("api.json total %d != %d enumerated methods", api.Total, n)
	}

	t.Logf("%d methods, %d exercised, %d skipped", n, len(covered), len(skip))
}

var intlRE = regexp.MustCompile(`Intl|toLocale|Temporal`)

// envSurface is the number of occurrences, in the en bundle, of each host-environment API that QuickJS or
// a bare V8 context may lack. A Faker upgrade that adds a new dependence changes a count and fails here,
// pointing at the methods to examine.
var envSurface = map[string]int{
	"Intl.": 0, "Temporal": 0, "localeCompare": 0, "toLocale": 1, "TextEncoder": 1, "btoa": 1, "atob": 0,
	"structuredClone": 0, "Buffer": 6,
}

// TestNoIntlDependence is test A10. No method may raise an error that mentions Intl, toLocale* or Temporal
// unless known_divergences.yaml lists it under category "intl"; every "intl" entry must describe a real
// divergence; and the bundle's host-API surface must match the recorded one. QuickJS has no Intl, but it
// ignores toLocaleString options silently rather than raising, which is why the surface scan matters.
func TestNoIntlDependence(t *testing.T) {
	cases, err := conformance.Load(conformance.PerMethod)
	if err != nil {
		t.Fatal(err)
	}

	known, err := conformance.LoadDivergences()
	if err != nil {
		t.Fatal(err)
	}

	listed := map[string]bool{}

	for _, d := range known {
		if d.Category == "intl" {
			listed[d.Method] = true
		}
	}

	hits := map[string]string{}
	seen := map[string]bool{}

	for _, c := range cases {
		key := c.Method() + "#" + c.ID[strings.Index(c.ID, "#"):strings.Index(c.ID, "@")]
		if seen[key] {
			continue
		}

		seen[key] = true
		e := fakerjs.NewEngine(42)

		for _, cs := range c.Calls {
			for range 20 {
				_, err := e.Generate(cs.M, cs.A)

				var fe *fakerjs.Error
				if errors.As(err, &fe) && fe.Class == fakerjs.ClassJSException && intlRE.MatchString(fe.Msg) {
					hits[cs.M] = fe.Msg
				}
			}
		}

		e.Close()
	}

	for m, msg := range hits {
		if !listed[m] {
			t.Errorf("%s raised an Intl-related error (%q) but is not in known_divergences.yaml with category intl", m, msg)
		}
	}

	outs := conformance.Evaluate(t, nativeFactory, conformance.PerMethod)

	for _, d := range known {
		if d.Category != "intl" {
			continue
		}

		real := false

		for _, o := range outs {
			if d.Matches(o.Case) && o.Disposition == conformance.Divergent {
				real = true
			}
		}

		if !real {
			t.Errorf("intl entry for %s no longer diverges", d.Method)
		}
	}

	bundle, err := os.ReadFile(filepath.Join(conformance.Root(), "bundle", "faker.en.min.js"))
	if err != nil {
		t.Fatal(err)
	}

	keys := make([]string, 0, len(envSurface))
	for k := range envSurface {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		if got := strings.Count(string(bundle), k); got != envSurface[k] {
			t.Errorf("bundle references %q %d times, recorded %d: examine the methods that use it and update envSurface", k, got, envSurface[k])
		}
	}
}
