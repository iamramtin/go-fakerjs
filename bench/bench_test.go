package bench

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/iamramtin/go-fakerjs"
	"github.com/iamramtin/go-fakerjs/conformance"
	"github.com/iamramtin/go-fakerjs/internal/wasi"
	"github.com/iamramtin/go-fakerjs/internal/wasm"
)

func mustEngine(b testing.TB, f engineFactory, seed int64) conformance.Engine {
	b.Helper()

	e, err := f.new(seed)
	if err != nil {
		b.Fatalf("%s: %v", f.name, err)
	}

	return e
}

type monotonic struct{ h *wasi.Host }

func (m monotonic) Xmonotonic_ns() int64 { return m.h.MonotonicNS() }

// E1: cold start including the first Generate.
func BenchmarkNewEngine(b *testing.B) {
	for _, f := range engines {
		b.Run(f.name, func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				e := mustEngine(b, f, 42)
				if _, err := e.Generate("person.firstName", nil); err != nil {
					b.Fatal(err)
				}

				e.Close()
			}
		})
	}
}

// E1: instantiating the Wasm snapshot alone (native only): allocate memory, copy the data segments, build
// the function table. No entropy, seeding or Generate.
func BenchmarkNewFromSnapshot(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		h := wasi.New(nil)
		_ = wasm.New(monotonic{h}, h)
	}
}

var singleMethods = []string{"person.firstName", "internet.email", "location.streetAddress", "lorem.sentence", "string.uuid", "number.int"}

// E2: one boundary crossing per value.
func BenchmarkGenerateSingle(b *testing.B) {
	for _, f := range engines {
		for _, m := range singleMethods {
			b.Run(f.name+"/"+m, func(b *testing.B) {
				e := mustEngine(b, f, 42)
				defer e.Close()

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					if _, err := e.Generate(m, nil); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// E3: n values per crossing; reports amortised ns per value.
func BenchmarkGenerateN(b *testing.B) {
	for _, f := range engines {
		for _, m := range singleMethods {
			for _, n := range []int{1, 10, 100, 1000} {
				b.Run(fmt.Sprintf("%s/%s/n=%d", f.name, m, n), func(b *testing.B) {
					e := mustEngine(b, f, 42)
					defer e.Close()

					b.ReportAllocs()
					b.ResetTimer()

					for i := 0; i < b.N; i++ {
						if _, err := e.GenerateN(m, nil, n); err != nil {
							b.Fatal(err)
						}
					}

					b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/value")
				})
			}
		}
	}
}

// row20 is the fixed 20-column row: the first 20 of the 22 workload paths, in the golden
// file's order (the last two, phone.number and string.alpha, are left out). Fixed so every run and every
// engine produces the same workload.
var row20 = []struct {
	m string
	a any
}{
	{"color.human", nil},
	{"commerce.department", nil},
	{"commerce.productDescription", nil},
	{"commerce.productMaterial", nil},
	{"commerce.productName", nil},
	{"company.name", nil},
	{"date.recent", nil},
	{"date.timeZone", nil},
	{"finance.currencyCode", nil},
	{"internet.email", nil},
	{"internet.url", nil},
	{"location.city", nil},
	{"location.country", nil},
	{"location.state", nil},
	{"location.streetAddress", nil},
	{"location.zipCode", nil},
	{"lorem.sentence", nil},
	{"lorem.word", nil},
	{"person.firstName", nil},
	{"person.lastName", nil},
}

const batchRows = 1000

// produceRows generates rows with one call per cell (the unbatched pattern) or column-batched.
func produceRows(b testing.TB, e conformance.Engine, rows int, batched bool) int {
	sink := 0

	if !batched {
		for range rows {
			var sb strings.Builder

			for _, c := range row20 {
				v, err := e.Generate(c.m, c.a)
				if err != nil {
					b.Fatal(err)
				}

				sb.WriteString(v)
			}

			sink += sb.Len()
		}

		return sink
	}

	for done := 0; done < rows; {
		n := min(batchRows, rows-done)

		cols := make([][]string, len(row20))
		for i, c := range row20 {
			vs, err := e.GenerateN(c.m, c.a, n)
			if err != nil {
				b.Fatal(err)
			}

			cols[i] = vs
		}

		for r := range n {
			var sb strings.Builder
			for i := range cols {
				sb.WriteString(cols[i][r])
			}

			sink += sb.Len()
		}

		done += n
	}

	return sink
}

// E4: rows/sec, single threaded. One op is one 20-column row.
func BenchmarkRow20(b *testing.B) {
	for _, f := range engines {
		for _, mode := range []string{"percall", "batched"} {
			b.Run(f.name+"/"+mode, func(b *testing.B) {
				e := mustEngine(b, f, 42)
				defer e.Close()

				b.ReportAllocs()
				b.ResetTimer()
				produceRows(b, e, b.N, mode == "batched")
				b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "rows/s")
			})
		}
	}
}

// E4: rows/sec with k engines on k goroutines (batched). b.N rows are shared between workers.
func BenchmarkRow20Parallel(b *testing.B) {
	sizes := []int{1, 2, 4, 8}
	if g := runtime.GOMAXPROCS(0); g != 1 && g != 2 && g != 4 && g != 8 {
		sizes = append(sizes, g)
	}

	for _, f := range engines {
		for _, mode := range []string{"percall", "batched"} {
			for _, k := range sizes {
				b.Run(fmt.Sprintf("%s/%s/pool=%d", f.name, mode, k), func(b *testing.B) {
					es := make([]conformance.Engine, k)
					for i := range es {
						es[i] = mustEngine(b, f, int64(i+1))
						defer es[i].Close()
					}

					b.ReportAllocs()

					var next atomic.Int64

					total := int64(b.N)
					b.ResetTimer()

					var wg sync.WaitGroup
					for i := range k {
						wg.Add(1)
						go func(e conformance.Engine) {
							defer wg.Done()

							for {
								start := next.Add(batchRows) - batchRows
								if start >= total {
									return
								}

								produceRows(b, e, int(min(batchRows, total-start)), mode == "batched")
							}
						}(es[i])
					}

					wg.Wait()
					b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "rows/s")
				})
			}
		}
	}
}

// E4 for the public Pool type (native only): borrow/release per batch.
func BenchmarkRow20Pool(b *testing.B) {
	for _, mode := range []string{"percall", "batched"} {
		for _, k := range []int{1, 2, 4, 8} {
			b.Run(fmt.Sprintf("%s/pool=%d", mode, k), func(b *testing.B) {
				p := fakerjs.NewPool(k, 42, fakerjs.WithRefDate(benchRef))
				defer p.Close()

				b.ReportAllocs()

				var next atomic.Int64

				total := int64(b.N)
				b.ResetTimer()

				var wg sync.WaitGroup
				for range k {
					wg.Go(func() {
						for {
							start := next.Add(batchRows) - batchRows
							if start >= total {
								return
							}

							e, release := p.Get()
							produceRows(b, e, int(min(batchRows, total-start)), mode == "batched")
							release()
						}
					})
				}

				wg.Wait()
				b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "rows/s")
			})
		}
	}
}

// E5: resident memory per engine and for a pool of 8 (steady state after one Generate each).
func TestResidentMemoryPerEngine(t *testing.T) {
	rss := func() uint64 {
		runtime.GC()

		b, err := os.ReadFile("/proc/self/statm")
		if err != nil {
			return 0
		}

		var size, res uint64
		if _, err := fmt.Sscan(string(b), &size, &res); err != nil {
			t.Fatalf("parse /proc/self/statm %q: %v", b, err)
		}

		return res * uint64(os.Getpagesize())
	}
	for _, f := range engines {
		before := rss()

		const n = 8

		es := make([]conformance.Engine, n)
		for i := range es {
			es[i] = mustEngine(t, f, int64(i+1))
			if _, err := es[i].Generate("person.firstName", nil); err != nil {
				t.Fatal(err)
			}
		}

		after := rss()
		t.Logf("RESULT %-14s pool(8) RSS %6.1f MB, per engine %6.1f MB", f.name, float64(after-before)/1e6, float64(after-before)/n/1e6)

		for _, e := range es {
			e.Close()
		}
	}
}
