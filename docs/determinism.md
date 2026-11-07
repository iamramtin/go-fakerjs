# Determinism

An engine's output is a function of its seed, its reference date, and the order of its calls. This page states each rule, where the output matches Faker.js, and where it doesn't. Conformance figures come from the Linux VM run in [`results/2026-10-18-linux-arm64-vm/`](../results/README.md), summarised in the [findings](../FINDINGS.md#51-conformance).

## Seeds

- **Non-zero seeds.** The `int64` seed is converted to `float64` with round-to-nearest-even and passed to `faker.seed(n)`. V8 makes the same conversion when it parses `faker.seed(<int64 literal>)`, so seeds above 2^53 that round to the same `float64` give the same sequence in both. The golden data includes seeds `1`, `42`, `2147483647`, `4294967296`, and `9007199254740991`, and the edge cases `-1`, 2^53+1, and the `int64` maximum and minimum.
- **Seed 0.** `faker.seed()` is called with no argument, so Faker.js draws its own seed from `Math.random`. Before that, 32 bytes from `crypto/rand` are mixed into `Math.random`'s state, and the contract tests check that two fresh seed-0 engines don't agree. Without that step every engine would start from the state saved in the snapshot ([`Math.random` reseeding](../FINDINGS.md#33-mathrandom-reseeding)).
- **Reference date.** `WithRefDate(t)` calls `faker.setDefaultRefDate`. Without it, `date.recent`, `date.past`, `date.future`, `date.soon`, and the other relative date methods count from the wall clock, so their output changes from day to day under a fixed seed.
- **Host clock.** `WithClock` replaces the clock the module sees. It exists so the tests can show that seeded output doesn't depend on the host clock.

## Sequences

- **One sequence per engine.** Each engine owns its Faker.js state. Calls on one engine are serialised by a mutex, so callers that share an engine receive values in the order they acquire the lock. Output is reproducible when each engine serves one logical stream.
- **Batches.** `GenerateN(method, arg, n)` returns the same values as `n` successive calls to `Generate` with the same argument.
- **Order.** Three 5,000-call sequences interleave the 22 workload methods with 40 other method and argument pairs. All three match Faker.js exactly, so the engine consumes the PRNG in Faker's order.
- **Closing.** `Close` is final. Every later call fails with `ErrClosed`, instead of starting a fresh engine whose sequence would begin again from the seed.
- **Poisoning.** A poisoned engine fails every call with `ErrPoisoned`. Its sequence can't be resumed, so a replacement starts again from its own seed.

## Pools

`NewPool(size, seed)` creates `size` engines. Engine `i` is seeded with `DeriveSeed(seed, i)`:

- index 0 uses `seed` itself;
- index `i > 0` uses `seed + i * 0x9E3779B97F4A7C15`, with wrapping `int64` arithmetic, and a result of 0 becomes 1, so a derived engine never falls back to a random seed;
- a `seed` of 0 stays 0 for every index.

`Get` hands out whichever engine is free, so which engine serves which goroutine depends on scheduling. For a reproducible result, give each stream its own engine. When a borrowed engine is poisoned, the pool replaces it on release with a new engine seeded with `DeriveSeed(seed, size + n)`, where `n` is 1 for the first replacement and grows by one with each, so a pool is reproducible only while no engine is poisoned.

## Platforms

The golden dump is the SHA-256 of the engine's output over the whole golden set (`TestWriteGoldenDump`). CI computes it on linux/amd64, linux/arm64, darwin/arm64, and windows/amd64 and fails unless all four are equal ([cross-architecture](../FINDINGS.md#53-build-and-reproducibility)).

## Compatibility with Faker.js

The oracle is Faker.js on Node 22.23.3 (V8 12.4) under `TZ=UTC`, with reference date 2026-01-01 UTC. Before it loads Faker.js, the generator removes the globals Node adds to the V8 environment: `btoa`, `atob`, `TextEncoder`, `TextDecoder`, `structuredClone`, `URL`, `URLSearchParams`, `Buffer`, and `crypto` ([`golden/gen.mjs`](../golden/gen.mjs)). The oracle therefore sees the same bare environment as the engine.

| Category | Cases | Exact | Catalogued divergence |
| --- | ---: | ---: | ---: |
| Per-method | 2,240 | 2,235 | 5 |
| Workload, 22 methods | 220 | 220 | 0 |
| Interleaved sequences | 3 | 3 | 0 |
| Seed edge cases | 20 | 20 | 0 |
| `String()` coercion | 52 | 52 | 0 |
| Errors, by class | 37 | 37 | 0 |

The tests run in strict mode against [`golden/known_divergences.yaml`](../golden/known_divergences.yaml): an unlisted divergence fails, and so does a listed entry that no longer diverges.

## Differences from Faker.js

- **`Intl`.** `finance.amount` with `autoFormat: true` calls `Number.prototype.toLocaleString(undefined, {minimumFractionDigits: dec})`. QuickJS-NG has no `Intl`, and its `toLocaleString` ignores the options, so `$20.550` becomes `$20.55`. All other digits are equal. This is the only catalogued divergence (5 cases).
- **Web-platform globals.** `internet.jwt`, and `image.dataUri` with `type: 'svg-base64'`, throw `ReferenceError: TextEncoder is not defined`. `image.dataUri` with no argument throws whenever Faker picks the base64 type. The oracle throws too, so these cases count as exact, but Faker.js in a stock Node process returns a value.
- **Time zone.** QuickJS prints `Date#toString` without a zone name, and the engine appends ` (Coordinated Universal Time)`. The output is the same under any host `TZ`. V8 prints the host's zone name and offset, so the two agree only under `TZ=UTC` ([UTC `Date` shim](../FINDINGS.md#34-the-utc-date-shim)).
- **One argument per call.** The module passes one JSON value. The second parameter of `helpers.arrayElements`, `helpers.mustache`, and `helpers.uniqueArray` can't be given, and no method can receive a function, which `helpers.multiple`, `helpers.maybe`, and `helpers.uniqueArray` accept.
- **Objects.** `String()` turns an object into `[object Object]`. In the golden data this affects `airline.airline`, `airline.airplane`, `airline.airport`, `finance.currency`, `location.language`, `science.chemicalElement`, and `science.unit`. Faker.js returns the object itself.
- **Latent engine differences.** `golden/probes.js` runs 73 probes in QuickJS-NG and V8, and 22 differ. Two reach golden data: `Date#toString`, which the shim corrects, and `toLocaleString` with options, which is the `Intl` divergence. The rest don't appear in any tested output: `Intl` is absent, `Array#sort` with an inconsistent or random comparator orders differently, the locale-formatted `Date` methods differ, one trigonometry probe differs in the last digits, and error message text differs while error classes match.
