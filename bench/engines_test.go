package bench

import (
	"time"

	"github.com/iamramtin/go-fakerjs"
	"github.com/iamramtin/go-fakerjs/bench/qjsbase"
	"github.com/iamramtin/go-fakerjs/conformance"
)

var benchRef = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type engineFactory struct {
	name string
	new  func(seed int64) (conformance.Engine, error)
}

// engines lists every implementation the benchmarks compare. v8go registers itself from v8go_test.go
// under the `v8go` build tag when its static V8 library is available.
var engines = []engineFactory{
	{"native", func(seed int64) (conformance.Engine, error) {
		return fakerjs.NewEngine(seed, fakerjs.WithRefDate(benchRef)), nil
	}},
	{"baseline-qjs", func(seed int64) (conformance.Engine, error) {
		return qjsbase.New(seed, benchRef)
	}},
}
