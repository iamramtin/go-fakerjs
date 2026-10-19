package fakerjs

import (
	"testing"
	"time"
)

// TestLeakWasmMemory is the Wasm half of stress.TestLeak. The module's linear memory is not reachable from the
// stress package, so this test runs the same workload and requires the linear memory to keep one size from the
// first call to the last.
func TestLeakWasmMemory(t *testing.T) {
	calls := 1_000_000

	switch {
	case raceEnabled:
		calls = 10_000 // the race detector makes a million calls run past the CI budget
	case testing.Short():
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

	e := NewEngine(42, WithRefDate(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	defer e.Close()

	pages := func() int { return len(*e.inst.m.Xmemory().Slice()) / 65536 }

	var first int

	for i := range calls {
		m := methods[i%len(methods)]
		if _, err := e.Generate(m.m, m.a); err != nil {
			t.Fatal(err)
		}

		if i == 0 {
			first = pages()
		}

		if i%100_000 == 0 {
			t.Logf("call=%7d wasm=%d pages (%d KiB)", i, pages(), pages()*64)
		}

		if p := pages(); p != first {
			t.Fatalf("call %d: Wasm linear memory grew from %d to %d pages", i, first, p)
		}
	}

	t.Logf("call=%7d wasm=%d pages (%d KiB), constant since call 0", calls, pages(), pages()*64)
}
