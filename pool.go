package fakerjs

import "sync"

// DeriveSeed returns the seed of engine index in a pool seeded with base: index 0 uses base itself and
// index i>0 uses base + i*0x9E3779B97F4A7C15 (wrapping int64 arithmetic), skipping 0 so that a derived
// engine never falls back to Faker's non-deterministic seed. A base of 0 stays 0 for every index.
func DeriveSeed(base int64, index int) int64 {
	if base == 0 {
		return 0
	}

	const golden = -7046029254386353131 // 0x9E3779B97F4A7C15 as int64

	s := base + int64(index)*golden
	if s == 0 {
		s = 1
	}

	return s
}

// Pool is a fixed set of engines for parallel generation. Engine i is seeded with DeriveSeed(base, i), so a
// pool is reproducible as long as each engine is used by one logical stream. An engine that gets poisoned
// is replaced on Put with a fresh one; the replacement's sequence restarts, so determinism is only
// guaranteed while no engine is poisoned.
type Pool struct {
	mu     sync.Mutex
	base   int64
	opts   []Option
	free   chan *poolEntry
	size   int
	gen    int // replacement counter, used to derive replacement seeds
	closed bool
}

type poolEntry struct {
	idx int
	e   *Engine
}

// NewPool returns a pool of size engines (at least 1).
func NewPool(size int, base int64, opts ...Option) *Pool {
	if size < 1 {
		size = 1
	}

	p := &Pool{base: base, opts: opts, free: make(chan *poolEntry, size), size: size}
	for i := 0; i < size; i++ {
		p.free <- &poolEntry{idx: i, e: NewEngine(DeriveSeed(base, i), opts...)}
	}

	return p
}

// Size is the number of engines in the pool.
func (p *Pool) Size() int { return p.size }

// Get borrows an engine, blocking until one is free. The returned release function must be called once.
func (p *Pool) Get() (*Engine, func()) {
	ent := <-p.free
	return ent.e, func() { p.put(ent) }
}

func (p *Pool) put(ent *poolEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		ent.e.Close()
		return
	}

	if ent.e.Poisoned() {
		ent.e.Close()

		p.gen++
		ent.e = NewEngine(DeriveSeed(p.base, p.size+p.gen), p.opts...)
	}

	p.free <- ent
}

// Do borrows an engine for the duration of fn.
func (p *Pool) Do(fn func(*Engine) error) error {
	e, release := p.Get()
	defer release()

	return fn(e)
}

// Close closes every engine returned to the pool. Engines still borrowed are closed on release.
func (p *Pool) Close() {
	p.mu.Lock()
	p.closed = true
	n := len(p.free)
	p.mu.Unlock()

	for range n {
		ent := <-p.free
		ent.e.Close()

		p.free <- ent
	}
}
