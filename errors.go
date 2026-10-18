package fakerjs

import (
	"errors"
	"fmt"
)

// Error classes reported by [Error.Class] and by the conformance harness.
const (
	// ClassInvalidMethod marks a method string that is not a "module.method" reference.
	ClassInvalidMethod = "invalid-method"
	// ClassUnknownMethod marks a well-formed method that is not on the allowlist.
	ClassUnknownMethod = "unknown-method"
	// ClassJSException marks an exception thrown by Faker.js (or by the engine while running it).
	ClassJSException = "js-exception"
	// ClassInvalidArgument marks an argument that is not valid JSON or is too large.
	ClassInvalidArgument = "invalid-argument"
	// ClassResource marks a call stopped by a resource limit: deadline, module memory or result size.
	ClassResource = "resource-limit"
	// ClassEngine marks a failure of the engine itself: closed, poisoned or unable to start.
	ClassEngine = "engine"
)

var (
	// ErrClosed is returned by every call on an engine after Close. A closed engine never restarts.
	ErrClosed = errors.New("engine is closed")
	// ErrPoisoned is returned when a previous call left the module in an unknown state (a trap or a panic
	// in a host import). The engine cannot be repaired without losing its sequence; use a [Pool] or make a
	// new engine.
	ErrPoisoned = errors.New("engine poisoned")
	// ErrDeadline is returned when a call ran past its timeout and was interrupted.
	ErrDeadline = errors.New("faker call exceeded its deadline")
	// ErrOutOfMemory is returned when a call exceeded the module's memory limit.
	ErrOutOfMemory = errors.New("faker call exceeded the Wasm memory limit")
	// ErrResultTooLarge is returned when a call's results exceed the result size limit.
	ErrResultTooLarge = errors.New("faker call result exceeds the size limit")
	// ErrArgTooLarge is returned when a method or argument exceeds the size limit.
	ErrArgTooLarge = errors.New("faker call argument exceeds the size limit")
	// ErrUnknownMethod is returned for a "module.method" that is not an allowlisted Faker method.
	ErrUnknownMethod = errors.New("unknown faker method")
)

// Error describes a failed call.
type Error struct {
	// Class is one of the Class constants.
	Class string
	// Method is the module.method that failed, when known.
	Method string
	// Msg is the engine's description; for ClassJSException it is the JavaScript exception text.
	Msg string
	// Err is the sentinel the failure matches with errors.Is, when there is one.
	Err error
}

func (e *Error) Error() string {
	switch {
	case e.Class == ClassJSException:
		return fmt.Sprintf("failed to execute faker.%s: %s", e.Method, e.Msg)
	case e.Msg != "":
		return e.Msg
	case e.Err != nil:
		return e.Err.Error()
	}

	return e.Class
}

func (e *Error) Unwrap() error { return e.Err }

// ErrorClass implements the interface the conformance harness uses to compare error classes.
func (e *Error) ErrorClass() string { return e.Class }
