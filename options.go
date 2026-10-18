package fakerjs

import "time"

const (
	// DefaultTimeout bounds a single Generate or GenerateN call.
	DefaultTimeout = 30 * time.Second
	// DefaultMaxArgBytes bounds the JSON-encoded argument of a call.
	DefaultMaxArgBytes = 4 << 20
)

type config struct {
	refDate     time.Time
	timeout     time.Duration
	maxArgBytes int
	clock       func() time.Time
	// hostHook runs inside the monotonic_ns host import; tests use it to inject panics.
	hostHook func()
}

func defaultConfig() config {
	return config{timeout: DefaultTimeout, maxArgBytes: DefaultMaxArgBytes}
}

// Option configures an [Engine] or [Pool].
type Option func(*config)

// WithRefDate pins Faker's default reference date (faker.setDefaultRefDate). Without it, date.recent,
// date.past, date.future and friends are relative to the wall clock and are not reproducible across days
// even with a fixed seed.
func WithRefDate(t time.Time) Option {
	return func(c *config) { c.refDate = t }
}

// WithTimeout bounds each Generate and GenerateN call. A call that runs longer is interrupted and fails
// with [ErrDeadline]. Zero or negative disables the bound.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithMaxArgBytes bounds the JSON-encoded argument of a call; larger arguments fail with
// [ErrArgTooLarge] before reaching the module.
func WithMaxArgBytes(n int) Option {
	return func(c *config) { c.maxArgBytes = n }
}

// WithClock replaces the wall clock the module sees through WASI. It exists so tests can prove that output
// does not depend on the host clock; production code should not need it.
func WithClock(now func() time.Time) Option {
	return func(c *config) { c.clock = now }
}
