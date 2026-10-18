package conformance_test

import (
	"testing"
	"time"

	"github.com/iamramtin/go-fakerjs"
	"github.com/iamramtin/go-fakerjs/conformance"
)

// nativeFactory is the engine under test: this package.
var nativeFactory = conformance.Factory{
	Name:   "native-en",
	Strict: true,
	New: func(seed int64, ref time.Time) (conformance.Engine, error) {
		return nativeEngine{fakerjs.NewEngine(seed, fakerjs.WithRefDate(ref))}, nil
	},
}

// nativeEngine adapts Close to the harness signature (identical here, kept for clarity).
type nativeEngine struct{ *fakerjs.Engine }

func TestWorkload22Exact(t *testing.T)           { conformance.Workload22Exact(t, nativeFactory) }
func TestInterleavedSequencesExact(t *testing.T) { conformance.InterleavedExact(t, nativeFactory) }
func TestPerMethodSeeded(t *testing.T)           { conformance.PerMethodSeeded(t, nativeFactory) }
func TestSeedEdges(t *testing.T)                 { conformance.SeedEdges(t, nativeFactory) }
func TestCoercionMatchesJSString(t *testing.T)   { conformance.CoercionMatchesJSString(t, nativeFactory) }
func TestErrorsMatchClass(t *testing.T)          { conformance.ErrorsMatchClass(t, nativeFactory) }
