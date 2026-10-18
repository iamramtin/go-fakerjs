//go:build baselines

package bench

import (
	"testing"
	"time"

	"github.com/iamramtin/go-fakerjs/bench/qjsbase"
	"github.com/iamramtin/go-fakerjs/conformance"
)

var qjsFactory = conformance.Factory{
	Name: "baseline-qjs",
	New: func(seed int64, ref time.Time) (conformance.Engine, error) {
		return qjsbase.New(seed, ref)
	},
}

func TestQJSWorkload22(t *testing.T)  { conformance.Workload22Exact(t, qjsFactory) }
func TestQJSInterleaved(t *testing.T) { conformance.InterleavedExact(t, qjsFactory) }
func TestQJSPerMethod(t *testing.T)   { conformance.PerMethodSeeded(t, qjsFactory) }
func TestQJSSeedEdges(t *testing.T)   { conformance.SeedEdges(t, qjsFactory) }
func TestQJSCoercion(t *testing.T)    { conformance.CoercionMatchesJSString(t, qjsFactory) }
func TestQJSErrors(t *testing.T)      { conformance.ErrorsMatchClass(t, qjsFactory) }
