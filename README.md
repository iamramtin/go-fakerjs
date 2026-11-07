<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/brand/go-fakerjs-dark.svg">
    <img src="docs/brand/go-fakerjs.svg" width="112" alt="go-fakerjs icon: rows of generated records, the first one started by a seed">
  </picture>
</p>

# go-fakerjs

*Faker.js, byte for byte, in a Go binary.*

go-fakerjs runs Faker.js inside a Go program, so Go code can generate the same fake data that Faker.js generates from the same seed, value for value. Faker.js and the QuickJS-NG interpreter are compiled to WebAssembly and then translated into Go source, so the package builds with `CGO_ENABLED=0`, needs no Wasm runtime, and imports nothing outside the standard library. Of 2,240 test cases across all 262 Faker.js methods, 2,235 match Faker.js byte for byte; the other 5 need `Intl`, which the interpreter doesn't have.

[![CI](https://github.com/iamramtin/go-fakerjs/actions/workflows/ci.yml/badge.svg)](https://github.com/iamramtin/go-fakerjs/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/iamramtin/go-fakerjs.svg)](https://pkg.go.dev/github.com/iamramtin/go-fakerjs)
[![Coverage](https://codecov.io/gh/iamramtin/go-fakerjs/graph/badge.svg)](https://codecov.io/gh/iamramtin/go-fakerjs)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Why it exists

I built it to get Faker.js output in Go, exact to the byte and reproducible from a seed, without cgo. A Go reimplementation of Faker can't match its output value for value, and each bridge in use today has a cost: v8go needs cgo and a prebuilt V8 library for every platform, wazero with QuickJS adds a third-party module and evaluates the Faker.js bundle every time an engine starts, and calling out to Node needs Node installed wherever the Go program runs. go-fakerjs replaces them with one Go module that runs Faker.js itself.

## Quick start

Requires Go 1.27.1 or later. Create a module and add go-fakerjs to it:

```sh
mkdir fakerdemo && cd fakerdemo
go mod init fakerdemo
go get github.com/iamramtin/go-fakerjs@latest
```

Save this as `main.go`:

```go
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/iamramtin/go-fakerjs"
)

func main() {
	// The same seed and reference date give the same values on every run.
	e := fakerjs.NewEngine(42, fakerjs.WithRefDate(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	defer e.Close()

	// show calls one Faker.js method. arg is nil for no argument, or any value encoding/json accepts.
	show := func(method string, arg any) {
		v, err := e.Generate(method, arg)
		if err != nil {
			fmt.Printf("%-22s error: %v\n", method, err)
			return
		}
		fmt.Printf("%-22s %s\n", method, v)
	}

	show("person.fullName", nil)                                    // text
	show("number.int", map[string]any{"max": 100})                  // a number
	show("datatype.boolean", nil)                                   // a boolean
	show("date.recent", nil)                                        // a Date
	show("helpers.arrayElements", []string{"red", "green", "blue"}) // an array
	show("airline.airline", nil)                                    // an object
	show("person.nickname", nil)                                    // not a Faker.js method

	// GenerateN returns several values from one call into the engine.
	codes, err := e.GenerateN("string.alpha", map[string]any{"length": 8}, 3)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(codes)
}
```

Run it:

```sh
go run .
```

```
person.fullName        Nikita Crist
number.int             5
datatype.boolean       false
date.recent            Wed Dec 31 2025 14:25:35 GMT+0000 (Coordinated Universal Time)
helpers.arrayElements  blue,green,red
airline.airline        [object Object]
person.nickname        error: unknown faker method "person.nickname"
[ljjpBwpF hptxOkAE cFidXYQp]
```

These are the values Faker.js prints on Node 22 with `faker.seed(42)` and the same reference date, under `TZ=UTC`, and the program prints them on every run. The first build compiles the generated Go that contains Faker.js, which took 9 s and peaked at 2.1 GB of memory on an Apple M5 with an empty build cache; later builds come from Go's build cache.

### Results

- **Text.** Every result is a Go `string`, converted the way JavaScript's `String()` converts it. Numbers and booleans become their text, a `Date` becomes its `Date#toString` text, an array is joined with commas, and an object becomes `[object Object]`. Seven methods return objects: `airline.airline`, `airline.airplane`, `airline.airport`, `finance.currency`, `location.language`, `science.chemicalElement`, and `science.unit`.
- **Arguments.** `nil` calls the method with no argument. Any other value is encoded with `encoding/json` and passed as the method's only argument, so a Go map, a slice, or a struct whose JSON field names match the Faker.js options all work.
- **Batches.** `GenerateN(method, arg, n)` returns the same `n` values as `n` calls to `Generate`, in one call into the engine.
- **Methods.** A method is named `module.method`, as in the [Faker.js API reference](https://fakerjs.dev/api/); every one of the 262 is listed in [`golden/api.json`](golden/api.json).

### Errors

`Generate` and `GenerateN` return a `*fakerjs.Error` with one of six classes, except that `GenerateN` with a negative count returns a plain error. `fakerjs.IsClass(err, class)` tests the class, and `errors.Is` matches the sentinel errors (`ErrUnknownMethod`, `ErrDeadline`, `ErrOutOfMemory`, `ErrResultTooLarge`, `ErrArgTooLarge`, `ErrPoisoned`, and `ErrClosed`). Each message below is real output:

| Class | Call | `err.Error()` |
| --- | --- | --- |
| `ClassInvalidMethod` | `Generate("person", nil)` | `invalid faker method format: "person" (expected format: module.method)` |
| `ClassUnknownMethod` | `Generate("person.nickname", nil)` | `unknown faker method "person.nickname"` |
| `ClassJSException` | `Generate("internet.jwt", nil)` | `failed to execute faker.internet.jwt: ReferenceError: TextEncoder is not defined` |
| `ClassInvalidArgument` | `Generate("string.alpha", make(chan int))` | `failed to marshal faker argument: json: unsupported type: chan int` |
| `ClassResource` | `Generate("string.nanoid", map[string]any{"length": 5})` with `WithTimeout(100 * time.Millisecond)` | `faker call exceeded its deadline` |
| `ClassEngine` | any call after `Close` | `engine is closed` |

`string.nanoid` with `length: 5` never returns in Faker.js itself, which makes it a convenient deadline test. Each call is bounded by `WithTimeout` (default 30 s), and its JSON argument by `WithMaxArgBytes` (default 4 MiB). An engine stays usable after a resource-limit error, but one that hit a memory limit keeps the memory it grew until it is closed ([limits](FINDINGS.md#52-robustness)).

## Who it's for

- **Test fixtures.** `Engine.Generate` with a fixed seed gives the same values on every run, and the same values on linux/amd64, linux/arm64, darwin/arm64, and windows/amd64, which CI compares.
- **Seeded, reproducible data.** `NewEngine(seed, WithRefDate(t))` reproduces the Faker.js sequence for that seed, date methods included.
- **Seeding databases.** `Engine.GenerateN` returns a column of values in one call into the engine.
- **Load tests.** `NewPool(size, seed)` runs engines in parallel, each seeded with `DeriveSeed(seed, i)`. Throughput is in [performance](docs/performance.md).
- **Demos.** `NewEngine(0)` leaves Faker.js on a random seed, so each new engine gives new values.

## What it covers

Every module and method that Faker.js's en instance exposes is callable: 27 modules and 262 methods, enumerated from Faker.js itself into [`golden/api.json`](golden/api.json). The methods are tested with 448 argument variants, each run with 5 seeds and 200 calls per seed. The reference is Faker.js on Node 22.23.3 (V8 12.4) under `TZ=UTC`, with Node's web-platform globals removed so that Faker.js sees the same bare environment as the engine (Linux VM, [conformance results](FINDINGS.md#51-conformance)).

| Module | Methods | Cases | Exact | Differences from Faker.js |
| --- | ---: | ---: | ---: | --- |
| `airline` | 7 | 55 | 55 | `airline`, `airplane`, and `airport` return `[object Object]` |
| `animal` | 16 | 80 | 80 | |
| `book` | 6 | 30 | 30 | |
| `color` | 11 | 105 | 105 | |
| `commerce` | 9 | 65 | 65 | |
| `company` | 9 | 45 | 45 | |
| `database` | 5 | 25 | 25 | |
| `datatype` | 1 | 15 | 15 | |
| `date` | 11 | 135 | 135 | `Date` text always names UTC; on Node it names the host's time zone |
| `finance` | 20 | 150 | 145 | `amount` with `autoFormat: true` gives `$20.55` for `$20.550`, because there's no `Intl` (5 cases); `currency` returns `[object Object]` |
| `food` | 9 | 45 | 45 | |
| `git` | 5 | 40 | 40 | |
| `hacker` | 6 | 30 | 30 | |
| `helpers` | 18 | 195 | 195 | One argument per call, so the second parameter of `arrayElements`, `mustache`, and `uniqueArray` can't be given; no function arguments, so `multiple` and `maybe` can't take a callback |
| `image` | 7 | 65 | 65 | `dataUri` throws `ReferenceError: TextEncoder is not defined` for `type: 'svg-base64'`, and with no argument whenever Faker picks that type; stock Node returns a base64 URI |
| `internet` | 21 | 175 | 175 | `jwt` throws `ReferenceError: TextEncoder is not defined`; stock Node returns a token |
| `location` | 20 | 160 | 160 | `language` returns `[object Object]` |
| `lorem` | 9 | 120 | 120 | |
| `music` | 4 | 20 | 20 | |
| `number` | 7 | 115 | 115 | |
| `person` | 15 | 125 | 125 | |
| `phone` | 2 | 25 | 25 | |
| `science` | 2 | 10 | 10 | `chemicalElement` and `unit` return `[object Object]` |
| `string` | 12 | 165 | 165 | |
| `system` | 12 | 85 | 85 | |
| `vehicle` | 9 | 45 | 45 | |
| `word` | 9 | 115 | 115 | |
| **Total** | **262** | **2,240** | **2,235** | |

The `image.dataUri` and `internet.jwt` cases count as exact because the oracle lacks the same globals; they differ from Faker.js on a stock Node process, which provides them.

**Locales.** en only, from `@faker-js/faker/locale/en`, one of the 76 locales in the pinned Faker.js package. There is no option to select another.

## Determinism and compatibility

| go-fakerjs | Faker.js | Oracle |
| --- | --- | --- |
| v0.1.0 | 10.6.0 (en locale) | Node 22.23.3 (V8 12.4), `TZ=UTC`, web-platform globals removed |

[`tools.versions`](tools.versions) is the single source of the Faker.js version, and `make check-versions` fails if any other statement of it disagrees.

- **Seeds.** A non-zero `int64` seed is passed to `faker.seed(n)` after conversion to `float64` with round-to-nearest-even, which is what V8 does with `faker.seed(<int64 literal>)`. Seeds above 2^53 that round to the same `float64` give the same sequence, as they do in Faker.js.
- **Seed 0.** `faker.seed()` is called with no argument, after 32 bytes from `crypto/rand` are mixed into `Math.random`, so two seed-0 engines give different sequences.
- **Reference date.** Without `WithRefDate`, `date.recent`, `date.past`, `date.future`, and the other relative date methods count from the wall clock and change from day to day under a fixed seed.
- **One sequence per engine.** Each engine owns its sequence, and calls on one engine are serialised. Values are reproducible when one logical stream uses each engine. `Close` is final, so a sequence never restarts silently.
- **Pools.** Engine `i` of `NewPool(size, seed)` is seeded with `DeriveSeed(seed, i)`. A poisoned engine is replaced with a fresh one whose sequence starts again, so a pool is reproducible only while no engine is poisoned.
- **Order.** Three 5,000-call sequences that interleave the 22 workload methods with 40 other method and argument pairs match Faker.js exactly, which shows that the engine consumes the PRNG in Faker's order.

The engine differs from Faker.js where QuickJS-NG differs from V8 and Node:

- **`Intl`.** QuickJS-NG has none, so `finance.amount` with `autoFormat` drops trailing zeros. This is the only catalogued divergence, in [`golden/known_divergences.yaml`](golden/known_divergences.yaml); the tests fail on any divergence that isn't listed there.
- **Web-platform globals.** The engine has none of the globals Node adds, such as `TextEncoder`, so `internet.jwt` and base64 `image.dataUri` throw. The oracle removes the same globals before it runs Faker.js ([`golden/gen.mjs`](golden/gen.mjs)), so these cases match the oracle and differ from a stock Node process.
- **Time zone.** The engine always prints `Date` text in UTC and appends ` (Coordinated Universal Time)`, so its output is the same under any host `TZ`. V8 prints the host's zone, so the two agree only under `TZ=UTC`.
- **Latent differences.** Of 73 probes run in both engines, 22 differ. Two reach golden data (the `Date` text, which the engine corrects, and `toLocaleString` with options). The others are `Array#sort` with an inconsistent comparator, the locale-formatted `Date` methods, the last digits of one trigonometry result, and error message text, where the error classes still match ([probes](FINDINGS.md#51-conformance)).

[Determinism](docs/determinism.md) covers each rule in detail.

## Design notes

- **Faker.js itself.** go-fakerjs runs the published Faker.js bundle and doesn't reimplement it, so equality with Faker.js is tested rather than approximated, and a new Faker.js release is adopted by rebuilding the snapshot, with [`make faker-upgrade`](CONTRIBUTING.md#fakerjs-upgrades), instead of porting code. The cost is throughput: QuickJS-NG interprets the JavaScript, where V8 compiles it.
- **Initialised at build time.** Wizer evaluates the Faker.js bundle once while the snapshot is built and saves the resulting memory, so `NewEngine` costs 0.31 ms on an Apple M5, against 23.4 ms for wazero with QuickJS, which evaluates the bundle at start-up, and 4.75 ms for v8go.
- **Private engines.** Each engine runs its own copy of the translated module, about 4.6 MB resident (Linux VM, development build). Calls on one engine are serialised by a mutex, so parallel work needs more engines, which `Pool` provides.
- **An allowlist of methods.** The module lists Faker.js's 262 methods when the snapshot is built and rejects any other name before JavaScript runs, so `constructor`, `__proto__`, and `faker` are refused. Arguments are parsed as JSON and never evaluated as code.
- **Bounded calls.** A deadline interrupts a call from inside the module, and a call that exceeds the memory limits fails with an error. A trap or panic poisons the engine, and later calls return `ErrPoisoned` instead of running on unknown state.
- **Committed generated code.** `internal/wasm/snapshot_en.go` (6,395,677 bytes) and `snapshot_en.dat` (3,465,429 bytes) are in the repository. The snapshot adds 10.2 MB to a stripped binary, and a cold build takes 8.9 s and peaks at about 1.9 GB of RSS (Linux VM, [sizes](FINDINGS.md#53-build-and-reproducibility)).

go-fakerjs deliberately does not:

- **Evaluate caller code.** Only allowlisted `module.method` names run, with JSON arguments.
- **Give the module a host surface.** QuickJS-NG's `std` and `os` modules aren't compiled in, so the module has no filesystem, process, network, or timer access. Its only imports are clocks and the stdio calls libc needs ([module imports](wasm/README.md#sources-and-build)).
- **Restart a closed engine.** Every call after `Close` fails with `ErrClosed`, instead of starting a fresh engine whose sequence would begin again from the seed.
- **Compress the embedded data.** Go module zips are already deflated, so gzip would save almost nothing on download and would add a decompression step to the first engine of every process.
- **Adopt a major Faker.js release automatically.** `make faker-upgrade` refuses major versions, which are upgraded by hand.

It also doesn't select a locale other than en, return typed Go values, take more than one argument or a function argument per call, or provide `Intl`.

[How it works](docs/how-it-works.md) describes the build pipeline and a call from Go into the module.

## Performance

Apple M5, darwin/arm64, medians of 10 runs, 20-column rows with one call per value. The v8go column builds a script per call; [performance](docs/performance.md) also has v8go with precompiled scripts.

| | go-fakerjs | v8go | wazero with QuickJS |
| --- | ---: | ---: | ---: |
| Engine start-up, first value included | 0.31 ms | 4.75 ms | 23.4 ms |
| Rows/s, 1 engine | 2,100 | 25,339 | 677 |
| Rows/s, 10 engines | 9,652 | 111,128 | 727 |

Per call, go-fakerjs is 1.3× (`number.int`) to 55× (`lorem.sentence`) slower than v8go. [Performance](docs/performance.md) has the figures per method and their sources.

## FAQ

**How to use Faker.js in Go (golang)?** Add go-fakerjs with `go get github.com/iamramtin/go-fakerjs@latest`, create an engine with `fakerjs.NewEngine(seed)`, and call any Faker.js method by name, as in `e.Generate("person.fullName", nil)`. The [quick start](#quick-start) is a complete program with its output.

**Is go-fakerjs a Go port of Faker.js?** No. It runs the published Faker.js bundle itself, in the QuickJS-NG interpreter, compiled to WebAssembly and translated into Go source. Nothing is reimplemented, so the values are Faker.js's own, and a new Faker.js release is adopted by rebuilding the snapshot rather than by porting code.

**Does go-fakerjs give the same values as Faker.js for the same seed?** Yes, on the tested surface: 2,235 of 2,240 test cases across all 262 methods match Faker.js byte for byte under `TZ=UTC`. The other 5 are `finance.amount` with `autoFormat`, which needs `Intl`. `internet.jwt` and base64 `image.dataUri` throw, because they need `TextEncoder`, which Node provides and the interpreter doesn't ([determinism and compatibility](#determinism-and-compatibility)).

**Does it need cgo, Node, or a Wasm runtime?** No. It builds with `CGO_ENABLED=0` and imports only the standard library. Cross-compiled test binaries for linux/amd64, linux/arm64, and windows/amd64 link no dynamic libraries, and CI checks that the engine's output over the whole golden set is identical on linux/amd64, linux/arm64, darwin/arm64, and windows/amd64 ([portability](FINDINGS.md#53-build-and-reproducibility)).

**How fast is it?** On an Apple M5, an engine starts in 0.31 ms, against 4.75 ms for v8go, and produces about 2,100 20-column rows per second, against 25,339 for v8go. Per call it is 1.3× to 55× slower than v8go, because QuickJS-NG interprets the JavaScript that V8 compiles ([performance](docs/performance.md)).

**How does it compare with gofakeit?** [gofakeit](https://github.com/brianvoe/gofakeit) is written in Go with its own generators and data, so its values are its own. go-fakerjs runs Faker.js, so its values are Faker.js's, which matters when Go and JavaScript code must produce the same fixtures from one seed.

## More

- [How it works](docs/how-it-works.md): the build pipeline, a call from Go into the module, and how a Faker.js release is adopted.
- [Determinism](docs/determinism.md): seeds, reference dates, pools, and every difference from Faker.js.
- [Performance](docs/performance.md): start-up, per-call, and parallel figures against v8go and wazero.
- [Findings](FINDINGS.md): method, results, analysis, threats to validity, and replication.
- [Results](results/README.md): the raw output behind every published figure, by date and machine.
- [Contributing](CONTRIBUTING.md): toolchain, regeneration, tests, and [Faker.js upgrades](CONTRIBUTING.md#fakerjs-upgrades).
- [Discussions](https://github.com/iamramtin/go-fakerjs/discussions): questions, ideas, and feedback. Bugs go in [issues](https://github.com/iamramtin/go-fakerjs/issues).
- [Security](SECURITY.md): threat model and how to report a vulnerability.
- [Changelog](CHANGELOG.md): release notes.
- [C harness and Wasm build](wasm/README.md): what the module contains and how it is built.
- [Third-party notices](THIRD_PARTY_NOTICES.md): bundled components, tools, and their licences.

## AI assistance

Claude models were used for variation testing and measurements. [Findings](FINDINGS.md#13-ai-assistance) records which models and what each did.

## Credits

Faker.js (MIT), QuickJS-NG (MIT; QuickJS by Fabrice Bellard and Charlie Gordon), wasm2go, Wizer, Binaryen, wasi-sdk, and esbuild. [Versions and licences](THIRD_PARTY_NOTICES.md) are listed in full.

## Licence

Released under the [MIT licence](LICENSE). The generated snapshot also contains code under the licences listed in [third-party notices](THIRD_PARTY_NOTICES.md).
