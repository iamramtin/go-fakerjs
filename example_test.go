package fakerjs_test

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iamramtin/go-fakerjs"
)

// The examples print properties that hold for every Faker.js release (determinism, lengths, error classes),
// so they stay valid across upgrades. Exact values are checked against Faker.js by the conformance suite.

// refDate is pinned so that date output does not depend on the wall clock.
var refDate = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

func ExampleNewEngine() {
	a := fakerjs.NewEngine(42, fakerjs.WithRefDate(refDate))
	defer a.Close()

	b := fakerjs.NewEngine(42, fakerjs.WithRefDate(refDate))
	defer b.Close()

	// The same seed and reference date give the same sequence.
	x, err := a.Generate("person.firstName", nil)
	if err != nil {
		panic(err)
	}

	y, err := b.Generate("person.firstName", nil)
	if err != nil {
		panic(err)
	}

	fmt.Println(x == y)
	// Output: true
}

func ExampleEngine_Generate() {
	e := fakerjs.NewEngine(42)
	defer e.Close()

	// Arguments are JSON-encoded, so a Go map or struct with the same keys works.
	code, err := e.Generate("string.alpha", map[string]any{"length": 8})
	if err != nil {
		panic(err)
	}

	fmt.Println(len(code))
	// Output: 8
}

func ExampleEngine_GenerateN() {
	e := fakerjs.NewEngine(7, fakerjs.WithRefDate(refDate))
	defer e.Close()

	// One boundary crossing for three values; the result equals three successive Generate calls.
	names, err := e.GenerateN("person.firstName", nil, 3)
	if err != nil {
		panic(err)
	}

	fmt.Println(len(names))
	// Output: 3
}

func ExampleEngine_Generate_errors() {
	e := fakerjs.NewEngine(1)
	defer e.Close()

	_, err := e.Generate("person.notAMethod", nil)

	fmt.Println(fakerjs.IsClass(err, fakerjs.ClassUnknownMethod), errors.Is(err, fakerjs.ErrUnknownMethod))
	// Output: true true
}

func ExampleNewPool() {
	p := fakerjs.NewPool(4, 42, fakerjs.WithRefDate(refDate))
	defer p.Close()

	// Engine i is seeded with DeriveSeed(42, i). Do borrows an engine for the duration of the function.
	err := p.Do(func(e *fakerjs.Engine) error {
		email, err := e.Generate("internet.email", nil)
		if err != nil {
			return err
		}

		fmt.Println(strings.Contains(email, "@"))

		return nil
	})
	if err != nil {
		panic(err)
	}
	// Output: true
}

func ExampleWithRefDate() {
	// Without WithRefDate, date.recent is relative to the wall clock and changes from day to day.
	// With it, a seeded engine prints the same date on every run.
	e := fakerjs.NewEngine(42, fakerjs.WithRefDate(refDate))
	defer e.Close()

	when, err := e.Generate("date.recent", map[string]any{"days": 10})
	if err != nil {
		panic(err)
	}

	// The ten days before 15 January 2026 all fall in January 2026.
	fmt.Println(strings.Contains(when, "Jan") && strings.Contains(when, "2026"))
	// Output: true
}
