//go:build gcstress

package fakerjs

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/iamramtin/go-fakerjs/conformance"
)

// TestSnapshotGCStress replays the first calls of every workload-path golden case with a full QuickJS cycle
// collection before every module allocation. Only built into the scratch copy made by scripts/gcstress.sh.
func TestSnapshotGCStress(t *testing.T) {
	calls := 40
	if v, err := strconv.Atoi(os.Getenv("GCSTRESS_CALLS")); err == nil {
		calls = v
	}

	cases, err := conformance.Load(conformance.Workload22)
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for _, c := range cases {
		if c.Seed != "42" && c.Seed != "1" {
			continue
		}

		var seed int64
		seed, _ = strconv.ParseInt(c.Seed, 10, 64)
		e := NewEngine(seed, WithRefDate(conformance.RefDate), WithTimeout(10*time.Minute))

		// start, then switch the stress on for this instance only
		in, err := e.start()
		if err != nil {
			t.Fatal(err)
		}

		e.inst = in
		in.m.Xset_gc_stress(1)
		for i := 0; i < calls; i++ {
			got, err := e.Generate(c.Calls[0].M, c.Calls[0].A)
			if err != nil || got != c.R[i].Value {
				t.Fatalf("%s call %d under GC stress: got (%q, %v) want %q", c.ID, i, got, err, c.R[i].Value)
			}
			checked++
		}

		e.Close()
	}

	t.Logf("%d golden calls reproduced with a full GC before every module allocation", checked)
}
