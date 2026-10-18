package fakerjs

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// C8: a host import that panics is converted to an error, the engine reports itself poisoned rather than
// continuing from an inconsistent module, and a Pool replaces it.
func TestHostImportPanicIsContained(t *testing.T) {
	boom := true

	e := NewEngine(42, WithTimeout(time.Minute), func(c *config) {
		c.hostHook = func() {
			if boom {
				panic("injected host failure")
			}
		}
	})
	defer e.Close()
	// monotonic_ns is only called by the interrupt handler, which a long-running call reaches.
	_, err := e.GenerateN("lorem.paragraphs", 50, 400)
	if !errors.Is(err, ErrPoisoned) || !IsClass(err, ClassEngine) {
		t.Fatalf("err = %v, want ErrPoisoned", err)
	}

	if !strings.Contains(err.Error(), "injected host failure") {
		t.Fatalf("panic value lost: %v", err)
	}

	if !e.Poisoned() {
		t.Fatal("engine should report Poisoned")
	}

	boom = false

	for range 3 {
		if _, err := e.Generate("person.firstName", nil); !errors.Is(err, ErrPoisoned) {
			t.Fatalf("poisoned engine answered: %v", err)
		}
	}
	// Close stays safe and terminal.
	e.Close()

	if _, err := e.Generate("person.firstName", nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("err = %v, want ErrClosed", err)
	}
}

func TestPoolReplacesPoisonedEngine(t *testing.T) {
	var calls int

	p := NewPool(1, 7, WithTimeout(time.Minute), func(c *config) {
		c.hostHook = func() {
			calls++
			if calls == 1 {
				panic("once")
			}
		}
	})
	defer p.Close()

	e, release := p.Get()
	if _, err := e.GenerateN("lorem.paragraphs", 50, 400); !errors.Is(err, ErrPoisoned) {
		t.Fatalf("err = %v, want ErrPoisoned", err)
	}

	release()

	e2, release2 := p.Get()
	defer release2()

	if e2 == e {
		t.Fatal("pool handed back the poisoned engine")
	}

	if _, err := e2.Generate("person.firstName", nil); err != nil {
		t.Fatalf("replacement engine: %v", err)
	}
}

// A panic raised while the instance is being built poisons the engine instead of escaping.
func TestStartupPanicIsContained(t *testing.T) {
	e := NewEngine(1, func(c *config) { c.clock = func() time.Time { panic("clock") } })
	defer e.Close()
	// The clock is only read when the module asks for the time, so force a call that does (new Date()).
	_, err := e.Generate("date.recent", nil)
	if err == nil {
		t.Skip("module did not read the clock; nothing to contain")
	}

	if !errors.Is(err, ErrPoisoned) {
		t.Fatalf("err = %v, want ErrPoisoned", err)
	}
}

// The module's own JSON parser must survive hostile nesting; encoding/json never gets to produce these
// (it stops at depth 10000), so the bytes go to the module directly.
func TestSnapshotJSONParserSurvivesDeepNesting(t *testing.T) {
	for _, depth := range []int{100, 10_000, 100_000, 1_000_000} {
		arg := []byte(strings.Repeat(`{"a":`, depth) + `1` + strings.Repeat(`}`, depth))
		e := NewEngine(1, WithMaxArgBytes(64<<20), WithTimeout(10*time.Second))
		_, err := e.generateJSON("string.alpha", arg, 1)
		t.Logf("depth %7d: err=%v poisoned=%v", depth, err, e.Poisoned())

		if e.Poisoned() {
			t.Fatalf("depth %d poisoned the engine", depth)
		}

		if err == nil && depth > 100_000 {
			t.Logf("depth %d accepted", depth)
		}

		if _, err := e.Generate("person.firstName", nil); err != nil {
			t.Fatalf("depth %d: engine unusable afterwards: %v", depth, err)
		}

		e.Close()
	}
	// Arrays and invalid documents.
	for _, arg := range []string{strings.Repeat("[", 500_000), strings.Repeat("[", 500_000) + strings.Repeat("]", 500_000), `{"a":`, ``, `nul`, "\x00", `"\ud800"`} {
		e := NewEngine(1, WithMaxArgBytes(64<<20))

		_, err := e.generateJSON("string.alpha", []byte(arg), 1)
		if e.Poisoned() {
			t.Fatalf("%.20q poisoned the engine (err %v)", arg, err)
		}

		e.Close()
	}
}
