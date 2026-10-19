# FINDINGS: Faker.js from pure Go

This report records how go-fakerjs runs Faker.js from Go without cgo or a Wasm runtime, how closely it
reproduces Faker.js on V8, what it costs, and what is still unknown.
The current pin is Faker.js 10.6.0; every result below was measured with Faker.js 10.6.0.

Each figure names its source: a [committed raw file](results/README.md), or a command whose output
is shown here. Two platforms appear, and every figure is labelled with one of them:

- **Linux VM**: linux/arm64, 4 vCPU, 7 GB RAM, a virtual machine on the macOS host. Tests, builds, robustness
  and conformance figures. Raw files: `results/2026-10-18-linux-arm64-vm/` and `results/2026-10-17-linux-arm64-vm/`.
- **macOS host**: darwin/arm64, Apple M5. All published performance figures. Raw files:
  `results/2026-10-17-apple-m5/`. They were measured on the [development build](#54-performance) that preceded v0.1.0.

## Contents

1. [Abstract](#1-abstract)
2. [Motivation](#2-motivation)
3. [Approach and pipeline](#3-approach-and-pipeline)
4. [Test methodology](#4-test-methodology)
5. [Results](#5-results)
6. [Analysis](#6-analysis)
7. [Negative results](#7-negative-results)
8. [Threats to validity](#8-threats-to-validity)
9. [Future work](#9-future-work)
10. [Replication](#10-replication)
11. [Raw data index](#11-raw-data-index)
12. [Credits](#12-credits)
13. [AI assistance](#13-ai-assistance)

## 1. Abstract

go-fakerjs runs the real Faker.js from Go. Faker.js and QuickJS-NG are compiled to WebAssembly, pre-initialised
with Wizer so the Faker data is already loaded, and translated to Go source by wasm2go. Each `Engine` runs its own
private copy of the translated program. The package needs no cgo, no Wasm runtime, and no third-party module.

On the tested surface the output is byte-identical to Faker.js on V8 under `TZ=UTC`. Of 2,240 per-method cases
(262 methods), 2,235 match exactly; the other 5 share one cause, the missing `Intl` (Linux VM). All 220 cases of
a 22-path workload match, and so do three 5,000-call interleaved sequences, which shows that the PRNG is consumed
in Faker's order.

Throughput is the cost. On the macOS host, one engine produces a 20-column row 12.3× more slowly than v8go
building a script per call. Per method, the gap is 1.3× to 55×. Engine start-up is 15.9× faster than v8go's. The
gap comes mostly from [QuickJS's interpreter](#61-the-call-boundary-is-close-to-v8gos-the-gap-is-the-interpreter), not from the call boundary.

## 2. Motivation

The package was built while evaluating ways to produce Faker.js-identical, seed-reproducible fake data from Go
without cgo, so that tests and fixtures can rely on exact output and builds need no C++ toolchain.

## 3. Approach and pipeline

### 3.1 Pipeline and exact versions

```mermaid
flowchart LR
  A["@faker-js/faker (npm)"] --> B["esbuild 0.25.10: IIFE bundle"]
  B --> C["QuickJS-NG v0.17.0 + wasm/harness.c"]
  C --> D["clang 23.1.0, wasi-sdk 34: wasm32-wasip1 reactor"]
  D --> E["wasm-opt -O2, Binaryen version_132"]
  E --> F["Wizer 11.0.3: bundle evaluated into the image"]
  F --> G["wasm2go v0.4.12: -embed -unsafe"]
  G --> H["internal/wasm/snapshot_en.go + snapshot_en.dat"]
```

The versions are pinned in `scripts/pins.env` (tools) and `tools.versions` (Faker.js), and `make doctor` records
them in `tools.versions`: Go 1.27.1; clang 23.1.0 (wasi-sdk 34); Wizer 11.0.3; Binaryen version_132; wasm2go
v0.4.12; QuickJS-NG v0.17.0 at commit `6d46d07d04041b40f4f49eaa7fdebe44c314c699`. The golden data was generated
with Node 22.23.3 (V8 12.4) and esbuild 0.25.10.

Wizer 11.0.3's init export is `wizer-initialize`, and Wizer has no `--version` flag, so `doctor` records the pinned
version rather than asking the binary.

### 3.2 Why each stage exists

- **esbuild bundle.** Faker.js ships ES modules, which the module cannot load. esbuild produces one IIFE, with tree
  shaking, private properties mangled (`--mangle-props=^_`) and console calls removed. [Oracle O2](#41-three-oracles) checks
  that the bundle behaves like the npm package.
- **clang with pinned features.** QuickJS-NG is compiled for `wasm32-wasip1` as a reactor (no `main`). The feature
  set is fixed (bulk memory, sign extension, non-trapping float-to-int, multivalue, mutable globals) and reference
  types are disabled, because Wizer does not support them. Without the pin, the output would follow the
  compiler's defaults.
- **wasm-opt -O2.** It shrinks the module from 1,374,819 to [1,374,647 bytes](#53-build-and-reproducibility). It is kept so the
  pipeline is unchanged from the build that was benchmarked.
- **Wizer pre-initialisation.** Wizer runs the module's initialiser once, at build time, and saves the resulting
  memory. The bundle is parsed and evaluated then, not on every engine start. This is why `NewEngine` costs 0.30
  ms on the macOS host, against 22.9 ms for the wazero + QuickJS baseline, which loads the bundle at start-up.
- **wasm2go -embed.** Translates the image to Go, with no runtime dependency. The linear-memory data segments go
  to a `.dat` file embedded with `go:embed`; the code becomes Go functions over a byte slice.

### 3.3 Math.random reseeding

QuickJS seeds `Math.random` and its hash seed from the clock when it creates a context. During Wizer's
initialisation the module returns a fixed time from `gettimeofday` and `clock_gettime`, and sets `Math.random`'s
state to a constant. Faker draws its default seed from `Math.random` while the bundle loads, so without this step
every engine would share one default seed and the snapshot would differ per build.

At run time the host mixes 32 bytes of `crypto/rand` into `Math.random`, then calls `faker.seed(n)` for a non-zero
seed or `faker.seed()` for seed 0. The contract tests check that two fresh seed-0 engines do not agree.

### 3.4 The UTC Date shim

QuickJS prints `Date#toString` without a time-zone name; V8 prints ` (Coordinated Universal Time)` under UTC and the
host's zone name elsewhere. The module appends the UTC name, so `date.*` output matches V8 under `TZ=UTC`. It is a
stand-in for V8's ICU display names, not an implementation of them: under any other `TZ` V8 prints another name.

### 3.5 Method allowlist

At initialisation the module lists Faker's module objects and their function-valued methods, and keeps the 262
names (recorded in `golden/api.json`). A call names a method as `module.method`. The Go side checks the grammar
(ASCII letters and digits, at most 256 bytes) and the module checks it again, then rejects any name not in the
list before any JavaScript runs. `constructor`, `toString`, `__proto__` and `faker` are rejected this way; the
contract tests and the fuzz targets exercise it. Arguments are JSON values, parsed by the module, never evaluated as
code.

## 4. Test methodology

### 4.1 Three oracles

- **O1, the npm package.** `@faker-js/faker` run on Node 22.23.3, `TZ=UTC`, reference date 2026-01-01 UTC. This is
  the reference behaviour.
- **O2, the bundle.** The minified bundle the module embeds, run in a clean `vm` context. O1 equals O2 on every case
  (`meta.json`: `o1EqualsO2`), so bundling changed no behaviour.
- **O3, eval per call.** Textual evaluation of `faker.<method>(<JSON arg>)` in a `vm` context, as an embedding that
  compiles a script per value would do. It runs on a sample: 162 cases, 0 mismatches (`meta.json`).

O1 against O2 isolates bundling. O2 against the Go engine isolates the translation (wasm2go, Wizer, the shim). O3
isolates the calling convention: a mismatch that only O3 shows points at argument or coercion handling.

### 4.2 Golden generation

`golden/enumerate.mjs` lists the methods and checks them against the grammar. `golden/gen.mjs` evaluates every case
under the oracles and writes `golden/testdata/`:

- `permethod`: 448 method and argument variants, 5 seeds each (`1`, `42`, `2147483647`, `4294967296`,
  `9007199254740991`), 200 calls per seed: 448,000 calls.
- `workload22`: 22 paths of a representative schema, 10 seeds each, 1,000 calls per case: 220,000 calls.
- `interleaved`: three 5,000-call sequences that interleave the workload with 40 other method and argument pairs.
  A different consumption order of the PRNG would change every later value, so these sequences are the proof
  that the order is preserved.
- `seededge`: seeds `-1`, 2^53+1, and the int64 maximum and minimum, for five methods, 20 calls each.
- `coercion`: values that JavaScript's `String` renders in unusual ways (260 calls).
- `errors`: 37 calls that must fail with the same error class.

`make golden-update` regenerates the data, and `make golden-verify` regenerates it in memory and requires identical
bytes.

### 4.3 The divergence ratchet

`golden/known_divergences.yaml` lists every known difference with its root cause, category, and cases. The harness
runs in strict mode: an unlisted divergence fails, and so does an entry that no longer diverges. The catalogue
therefore always equals the measured behaviour.

### 4.4 Coverage completeness

`TestAPICoverageComplete` requires every enumerated method to be exercised by a per-method case or listed with a
reason in `golden/skipped.yaml` (empty today). A method in both, or in neither, fails the suite.

### 4.5 Robustness

Resource bombs (calls built to exhaust time or memory) must end in a bounded error with a usable engine.
`WithTimeout` deadlines interrupt runaway calls from inside the module. `FuzzGenerate` fuzzes the method and JSON
argument; `FuzzSnapshotArgs` feeds raw bytes to the module's JSON parser. GC stress runs with `GOGC=1`, forced
collections and `GODEBUG=gccheckmark=1`. The race detector covers the package, the conformance, contract and
stress suites.

## 5. Results

### 5.1 Conformance

Linux VM. Engine: the en snapshot, `go test ./conformance`. Raw file:
`results/2026-10-18-linux-arm64-vm/conformance-native-en.json`; tables from `scripts/tables.py conformance`.
Exact means byte-equal to O1. Normalised means equal after removing the date suffix; the en snapshot has none.
Divergent means a catalogued difference.

#### native-en: by category

| category | cases | exact | normalized | divergent | pass |
|---|---|---|---|---|---|
| coercion | 52 | 52 | 0 | 0 | 100.00% |
| errors | 37 | 37 | 0 | 0 | 100.00% |
| interleaved | 3 | 3 | 0 | 0 | 100.00% |
| per-method | 2240 | 2235 | 0 | 5 | 99.78% |
| seed-edge | 20 | 20 | 0 | 0 | 100.00% |
| workload-22 | 220 | 220 | 0 | 0 | 100.00% |

#### native-en: per-method cases by module

| module | cases | exact | normalized | divergent | pass |
|---|---|---|---|---|---|
| airline | 55 | 55 | 0 | 0 | 100.00% |
| animal | 80 | 80 | 0 | 0 | 100.00% |
| book | 30 | 30 | 0 | 0 | 100.00% |
| color | 105 | 105 | 0 | 0 | 100.00% |
| commerce | 65 | 65 | 0 | 0 | 100.00% |
| company | 45 | 45 | 0 | 0 | 100.00% |
| database | 25 | 25 | 0 | 0 | 100.00% |
| datatype | 15 | 15 | 0 | 0 | 100.00% |
| date | 135 | 135 | 0 | 0 | 100.00% |
| finance | 150 | 145 | 0 | 5 | 96.67% |
| food | 45 | 45 | 0 | 0 | 100.00% |
| git | 40 | 40 | 0 | 0 | 100.00% |
| hacker | 30 | 30 | 0 | 0 | 100.00% |
| helpers | 195 | 195 | 0 | 0 | 100.00% |
| image | 65 | 65 | 0 | 0 | 100.00% |
| internet | 175 | 175 | 0 | 0 | 100.00% |
| location | 160 | 160 | 0 | 0 | 100.00% |
| lorem | 120 | 120 | 0 | 0 | 100.00% |
| music | 20 | 20 | 0 | 0 | 100.00% |
| number | 115 | 115 | 0 | 0 | 100.00% |
| person | 125 | 125 | 0 | 0 | 100.00% |
| phone | 25 | 25 | 0 | 0 | 100.00% |
| science | 10 | 10 | 0 | 0 | 100.00% |
| string | 165 | 165 | 0 | 0 | 100.00% |
| system | 85 | 85 | 0 | 0 | 100.00% |
| vehicle | 45 | 45 | 0 | 0 | 100.00% |
| word | 115 | 115 | 0 | 0 | 100.00% |

**Workload and interleaved sequences.** 220 of 220 workload cases are exact with no allowlisted divergence, and
all 3 interleaved sequences are exact. This depends on the [UTC Date shim](#34-the-utc-date-shim).

**Baseline.** The wazero + QuickJS baseline has no shim. Linux VM, `make bench-baselines`, raw file
`results/2026-10-18-linux-arm64-vm/conformance-baseline-qjs.json`. Its workload and interleaved tests fail by
design, because they demand byte-exact output:

#### baseline-qjs: by category


| category | cases | exact | normalized | divergent | pass |
|---|---|---|---|---|---|
| coercion | 52 | 40 | 12 | 0 | 100.00% |
| errors | 37 | 37 | 0 | 0 | 100.00% |
| interleaved | 3 | 0 | 3 | 0 | 100.00% |
| per-method | 2240 | 2135 | 100 | 5 | 99.78% |
| seed-edge | 20 | 16 | 4 | 0 | 100.00% |
| workload-22 | 220 | 210 | 10 | 0 | 100.00% |

**Divergence catalogue.** One method diverges, in 5 cases:

| method | cases | root cause | category |
|---|---|---|---|
| `finance.amount` with `{autoFormat: true, dec: 3, ...}` | 5 (variant 1, every seed) | `autoFormat` calls `Number.prototype.toLocaleString(undefined, {minimumFractionDigits: dec})`. QuickJS-NG has no `Intl`, and its `toLocaleString` ignores the options, so `$20.550` becomes `$20.55`. All other digits are equal. | `intl` |

**QuickJS versus V8 probes.** `golden/probes.js` runs 73 probes in both engines. 51 give the same result and 22
differ (Linux VM; `node golden/probes.mjs`, then `go test -tags baselines -run TestQJSProbes` in `bench/`; raw file
`results/2026-10-18-linux-arm64-vm/probes.txt`). Two differences reach golden data: `Date#toString`, which the
shim fixes, and `toLocaleString` with options, the divergence above. The others are latent: `Intl` is absent
(test A10 scans for it), `Array#sort` with an inconsistent or random comparator orders differently, the
locale-formatted `Date` methods differ, one trigonometry probe differs in the last digits, and error message text
differs (error classes match).

**Host APIs the bundle uses.** `TestNoIntlDependence` counts them in the en bundle: `toLocale*` once
(`finance.amount`), `TextEncoder` once (`internet.jwt`), `btoa` once (`image.dataUri`), `Buffer` six times
(guarded). No `Intl`, `Temporal`, `localeCompare`, `atob` or `structuredClone`. `internet.jwt` and the base64 form
of `image.dataUri` throw under a bare V8 context too, and are recorded as expected errors.

### 5.2 Robustness

Linux VM, final build. Raw files in `results/2026-10-18-linux-arm64-vm/`.

| test | result | source |
|---|---|---|
| GC stress (`TestGCStress`) | 235,000 golden calls verified across 271 forced collections | `stress-verbose.txt` |
| Module GC stress | 1,760 golden calls reproduced with a full QuickJS collection before every module allocation (`scripts/gcstress.sh`, default `GCSTRESS_CALLS=40`). No `JSValue` is dropped or double-freed, which is why `wasm-opt --spill-pointers` is not used. | `guest-gcstress.txt` |
| Leak (`TestLeak`, 1,000,000 calls on one engine) | post-GC heap 6,060 KiB after warm-up and at the end (limit +2 MB); Wasm linear memory 92 pages (5,888 KiB) at every call (`TestLeakWasmMemory`) | `go test -v -run 'TestLeak' .` on linux/arm64, not yet in `results/` |
| Churn (`TestInstanceChurn`, 2,000 engines) | RSS 16 MB before, 20 MB after | `stress-verbose.txt` |
| Isolation (`TestIsolation`) | engines with different seeds, interleaved call by call, each reproduce their own golden sequence | `stress-verbose.txt` |
| Deadlines (`TestDeadline`) | `string.nanoid({length: 5})` never returns in Faker.js 10.6.0 itself. Timeouts of 100 ms, 500 ms and 2 s return `ErrDeadline` after 100 ms, 500 ms and 2 s, and the engine stays usable | `stress-verbose.txt` |
| Pool recovery (`TestPoolRecoversFromBomb`) | a pool replaces an engine poisoned by a bomb | `stress-verbose.txt` |
| Panic containment (`TestHostImportPanicIsContained`, `TestStartupPanicIsContained`) | a panic in a host import or during start-up poisons the engine; later calls fail with `ErrPoisoned` | `go test .` |
| Race detector, package and suites (`go test -race ./contract ./conformance .`) | pass: contract 128 s, conformance 861 s, root package 3 s | `race-root-contract-conformance.txt` |
| Race detector, stress at full budgets (`go test -race ./stress`, no `-short`) | pass in 2,649 s | `race-stress-full.txt` |
| Fuzz under `-race` (`FuzzGenerate`, 10 minutes) | 879,751 executions, 122 new interesting inputs, no crash, hang or failure | `fuzz-race-10m.txt` |
| Fuzz smoke (`make test-fuzz FUZZTIME=60s`: `FuzzGenerate` 60 s, `FuzzSnapshotArgs` 1 min) | both pass | `fuzz-smoke-60s.txt` |

Resource bombs (`TestResourceBombs`, timeout 5 s per call; raw file `bombs.json`). Peak RSS is the process's,
cumulative over the run:

| bomb | outcome | time (ms) | peak RSS (MB) | engine usable after |
|---|---|---|---|---|
| string.alpha length 2^30 | resource-limit:faker call exceeded its deadline | 5005 | 531 | yes |
| string.alpha length 2^24 | resource-limit:faker call exceeded its deadline | 5004 | 705 | yes |
| string.numeric length 1e9 | resource-limit:faker call exceeded its deadline | 5004 | 727 | yes |
| lorem.paragraphs 1e8 | resource-limit:faker call exceeded its deadline | 5003 | 727 | yes |
| lorem.words 1e9 | resource-limit:faker call exceeded its deadline | 5001 | 727 | yes |
| lorem.text via GenerateN 1e8 | resource-limit:faker call exceeded its deadline | 5002 | 727 | yes |
| person.firstName GenerateN 1e9 | resource-limit:faker call exceeded its deadline | 5000 | 727 | yes |
| helpers.fromRegExp a{1e8} | resource-limit:faker call exceeded the Wasm memory limit | 213 | 727 | yes |
| helpers.fromRegExp nested quantifiers | ok | 0 | 727 | yes |
| helpers.arrayElements giant count | ok | 656 | 741 | yes |
| helpers.shuffle 2^20 elements | ok | 1751 | 741 | yes |
| helpers.slugify 3 MiB | ok | 180 | 741 | yes |
| nested options JSON depth 10000 | ok | 6 | 741 | yes |
| nested options JSON depth 100000 | invalid-argument:exceeded max depth | 3 | 741 | yes |
| 50 MB args (default cap) | invalid-argument:faker call argument exceeds the size limit | 22 | 741 | yes |
| 50 MB args (cap lifted) | resource-limit:faker call exceeded the Wasm memory limit | 108 | 741 | yes |
| 50 MB method string | invalid-method | 186 | 741 | yes |
| helpers.fake escaped nesting depth 14 | js-exception | 0 | 741 | yes |
| helpers.fake braces nested 500 | ok | 0 | 741 | yes |
| helpers.fake braces nested 100000 | ok | 9 | 741 | yes |
| helpers.fake template 3 MiB | resource-limit:faker call exceeded the Wasm memory limit | 105 | 897 | yes |
| string.nanoid runaway loop | resource-limit:faker call exceeded its deadline | 5001 | 902 | yes |

Limits: module JavaScript heap 160 MiB (`ErrOutOfMemory`), WebAssembly memory 256 MiB (linker `--max-memory`),
result 64 MiB, argument 4 MiB by default (`WithMaxArgBytes`), method 256 bytes, default timeout 30 s. A call that
hits the memory limit grows the process while the WebAssembly memory grows (the largest step in the table above
is 280 MB), and the engine keeps that memory until it is closed. Recycle an engine after a resource-limit error.

### 5.3 Build and reproducibility

**Reproducibility.** `make snapshot-repro` builds the snapshot twice, the second time from a copy of the repository at
another absolute path, and compares the SHA-256 of the optimised wasm, the wizened wasm, the generated Go and the
data file (Linux VM, raw file `results/2026-10-18-linux-arm64-vm/repro.txt`):

```
REPRODUCIBLE [en]
df0e2899b2c7a0de9f43a355f9bbebd40f1711f7b51ed986b834c7a2a44de4b0  snapshot_en.opt.wasm
7e6fa9d40a64a43d89702bd4a7b83be4e3d98405b455b1bb50a5d9e6e7402176  snapshot_en.wizened.wasm
2f1a0f2bd30e0f92f737ecb7407cf64bb846c48ad86537020502890ca270e059  snapshot_en.go
d90e43df73555b8d48ae37728adf298870343bd66f6770ffea6b870fabe0d04d  snapshot_en.dat
```

The same pins rebuilt from scratch reproduce the development build's bundle, optimised and wizened wasm, and data
file byte for byte. The generated Go differs only by a removed `//go:build` line.

**Cross-architecture.** The golden dump is the SHA-256 of the engine's output over the whole golden set
(`TestWriteGoldenDump`). On the Linux VM it is `5c49fd041c8230fe2181e72807bbf961561bdedaf2ed8210a4f813b892bd58c4`
(`golden-dump-en-arm64.sha256`). The CI matrix computes it on linux/amd64, linux/arm64, darwin/arm64 and
windows/amd64 and fails unless all four are equal; `scripts/host/golden-hash.sh` records the macOS host's value.
CI confirmed that all four hashes are equal.

**Portability.** `make portability` cross-compiles the contract test binary for five platforms and lists the
dynamic libraries each links (Linux VM, raw file `portability.txt`). It does not run the foreign binaries:

```
BUILD ok   darwin/arm64 (0s, test binary 19M)
build/portability/contract-darwin-arm64: Mach-O, 2 dynamic libraries [/usr/lib/libSystem.B.dylib /usr/lib/libresolv.9.dylib]
BUILD ok   darwin/amd64 (1s, test binary 20M)
build/portability/contract-darwin-amd64: Mach-O, 2 dynamic libraries [/usr/lib/libSystem.B.dylib /usr/lib/libresolv.9.dylib]
BUILD ok   linux/amd64 (0s, test binary 21M)
build/portability/contract-linux-amd64: ELF, 0 dynamic libraries []
BUILD ok   linux/arm64 (1s, test binary 19M)
build/portability/contract-linux-arm64: ELF, 0 dynamic libraries []
BUILD ok   windows/amd64 (0s, test binary 21M)
build/portability/contract-windows-amd64.exe: PE, 0 dynamic libraries []
--- host tests (linux/arm64)
ok  	github.com/iamramtin/go-fakerjs/conformance	7.476s
ok  	github.com/iamramtin/go-fakerjs/contract	0.041s
SKIP docker: not installed, so linux/amd64 and linux/arm64 container runs did not happen
--- golden dump hashes by architecture (reports/golden-dump-*.sha256)
5c49fd041c8230fe2181e72807bbf961561bdedaf2ed8210a4f813b892bd58c4  golden-dump-en-arm64.sha256
dumps identical across: arm64
```

**Generated source and sizes** (Linux VM, this build; `scripts/snapshot.sh en` writes them):

| artefact | en |
|---|---|
| wasm from clang (`-O2`) | 1,374,819 B |
| after `wasm-opt -O2` | 1,374,647 B |
| Wizer snapshot (Faker evaluated) | 4,235,444 B |
| generated Go | 6,395,677 B |
| generated Go, lines | 228,011 |
| embedded data (`.dat`) | 3,465,429 B |
| Faker bundle (`faker.en.min.js`) | 416,501 B |

**Binary size and build cost.** `scripts/binsize.sh`: stripped binaries of a program that calls `NewEngine` once,
each built with an empty build cache (Linux VM, raw file `binsize.txt`). The en snapshot adds 10.2 MB to an
empty program; a cold build takes 8.9 s and peaks at about 1.9 GB of RSS.

```
target                  bytes    build_s       rss_MB
empty                 1245344        1.0          284
native-en            11468960        8.9         1950
baseline-qjs          7012615        3.7          277
```

**Compressing the data file.** Considered and rejected. A measurement in an earlier session of this port (Linux VM,
not kept as a raw file) found that Go module zips are already deflated, so gzip would save almost nothing on download while
adding a decompression step to the first engine of every process; zstd would need a third-party decoder. The
data file is embedded raw.

### 5.4 Performance

macOS host (Apple M5). Raw files: `results/2026-10-17-apple-m5/bench-all.txt` (`make bench`: native and the wazero
+ QuickJS baseline) and `bench-v8go.txt` (`make bench-v8go`: tommie/v8go v0.34.0, script built per call and
precompiled, plus native and baseline rows from the same run). `-count=10`; medians. Where a benchmark appears in
both files, the v8go run's value is used, so every ratio compares rows from the same run.

These figures come from the development build that preceded v0.1.0. It used the same pinned Faker.js 10.6.0 and
toolchain, and its engine code and snapshot are the same as v0.1.0's (only comments, a constant and a build tag
changed). `scripts/host/bench-all.sh` re-measures them on the final build.

Row20 is the first 20 of the 22 workload paths, one row per operation. "percall" is one `Generate` per cell,
"batched" is one `GenerateN` per column. Row20 with k engines shares the rows between k goroutines; its rows/s is
the aggregate.

#### Cold start (Apple M5, median of 10)

| benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| NewEngine/baseline-qjs | 22,881,023 | 21,788,370 | 1,748 |
| NewEngine/native | 299,130 | 6,056,667 | 509 |
| NewEngine/v8go-precompiled | 4,802,535 | 837,135 | 35 |
| NewEngine/v8go-script-per-call | 4,757,472 | 836,849 | 31 |
| NewFromSnapshot | 368,276 | 6,056,449 | 503 |

#### GenerateSingle (Apple M5, ns/op, median of 10)

| method | native | baseline-qjs | v8go script per call | v8go precompiled | native / v8go script |
|---|---|---|---|---|---|
| person.firstName | 6,178 | 34,554 | 1,101 | 829 | 5.6x |
| internet.email | 36,840 | 110,902 | 3,213 | 3,100 | 11.5x |
| location.streetAddress | 48,412 | 139,037 | 3,653 | 3,108 | 13.3x |
| lorem.sentence | 112,401 | 206,313 | 2,053 | 1,824 | 54.7x |
| string.uuid | 49,376 | 114,509 | 3,093 | 2,832 | 16.0x |
| number.int | 1,482 | 20,208 | 1,141 | 965 | 1.3x |

#### GenerateN amortised ns per value (Apple M5, median of 10)

| method | n | native | baseline-qjs | qjs / native |
|---|---|---|---|---|
| person.firstName | 1 | 6,276 | 35,626 | 5.7x |
| person.firstName | 10 | 6,033 | 14,177 | 2.3x |
| person.firstName | 100 | 5,988 | 11,608 | 1.9x |
| person.firstName | 1000 | 5,978 | 11,320 | 1.9x |
| internet.email | 1 | 37,326 | 113,067 | 3.0x |
| internet.email | 10 | 36,436 | 78,176 | 2.1x |
| internet.email | 100 | 36,344 | 76,438 | 2.1x |
| internet.email | 1000 | 36,502 | 73,711 | 2.0x |
| location.streetAddress | 1 | 48,568 | 141,471 | 2.9x |
| location.streetAddress | 10 | 48,210 | 105,948 | 2.2x |
| location.streetAddress | 100 | 49,104 | 102,024 | 2.1x |
| location.streetAddress | 1000 | 48,052 | 101,400 | 2.1x |
| lorem.sentence | 1 | 113,077 | 209,364 | 1.9x |
| lorem.sentence | 10 | 112,708 | 178,139 | 1.6x |
| lorem.sentence | 100 | 111,926 | 177,791 | 1.6x |
| lorem.sentence | 1000 | 112,534 | 175,166 | 1.6x |
| string.uuid | 1 | 50,102 | 114,580 | 2.3x |
| string.uuid | 10 | 51,422 | 95,280 | 1.9x |
| string.uuid | 100 | 49,728 | 93,176 | 1.9x |
| string.uuid | 1000 | 49,182 | 92,553 | 1.9x |
| number.int | 1 | 1,476 | 21,036 | 14.2x |
| number.int | 10 | 1,326 | 4,850 | 3.7x |
| number.int | 100 | 1,298 | 3,083 | 2.4x |
| number.int | 1000 | 1,300 | 2,898 | 2.2x |

#### Row20 (Apple M5): one Generate per cell (percall) vs GenerateN per column (batched)

| engine | mode | rows/s | ns/row | B/op | allocs/op |
|---|---|---|---|---|---|
| native | percall | 2,092 | 478,096 | 4,358 | 45 |
| native | batched | 2,209 | 452,764 | 4,143 | 25 |
| baseline-qjs | percall | 661 | 1,513,888 | 9,973,432 | 2,217 |
| baseline-qjs | batched | 1,197 | 835,311 | 17,702 | 23 |
| v8go-script-per-call | percall | 25,637 | 39,006 | 2,641 | 85 |
| v8go-script-per-call | batched | 28,285 | 35,354 | 2,967 | 85 |
| v8go-precompiled | percall | 29,385 | 34,031 | 1,777 | 45 |
| v8go-precompiled | batched | 33,481 | 29,868 | 2,103 | 45 |

#### Row20 with k engines on k goroutines (Apple M5, 10 cores, aggregate rows/s, median of 10)

| engine | mode | engines | rows/s | speed-up vs 1 engine |
|---|---|---|---|---|
| native | percall | 1 | 2,076 | 1.00x |
| native | percall | 2 | 3,876 | 1.87x |
| native | percall | 4 | 5,226 | 2.52x |
| native | percall | 8 | 7,130 | 3.44x |
| native | percall | 10 | 8,464 | 4.08x |
| native | batched | 1 | 2,083 | 1.00x |
| native | batched | 2 | 3,994 | 1.92x |
| native | batched | 4 | 5,630 | 2.70x |
| native | batched | 8 | 7,586 | 3.64x |
| native | batched | 10 | 9,176 | 4.40x |
| baseline-qjs | percall | 1 | 658 | 1.00x |
| baseline-qjs | percall | 2 | 669 | 1.02x |
| baseline-qjs | percall | 4 | 687 | 1.05x |
| baseline-qjs | percall | 8 | 698 | 1.06x |
| baseline-qjs | percall | 10 | 704 | 1.07x |
| baseline-qjs | batched | 1 | 1,142 | 1.00x |
| baseline-qjs | batched | 2 | 1,910 | 1.67x |
| baseline-qjs | batched | 4 | 3,158 | 2.77x |
| baseline-qjs | batched | 8 | 2,861 | 2.51x |
| baseline-qjs | batched | 10 | 2,764 | 2.42x |
| v8go-script-per-call | percall | 1 | 25,528 | 1.00x |
| v8go-script-per-call | percall | 2 | 42,199 | 1.65x |
| v8go-script-per-call | percall | 4 | 74,779 | 2.93x |
| v8go-script-per-call | percall | 8 | 103,985 | 4.07x |
| v8go-script-per-call | percall | 10 | 106,922 | 4.19x |
| v8go-script-per-call | batched | 1 | 26,874 | 1.00x |
| v8go-script-per-call | batched | 2 | 48,925 | 1.82x |
| v8go-script-per-call | batched | 4 | 94,366 | 3.51x |
| v8go-script-per-call | batched | 8 | 119,682 | 4.45x |
| v8go-script-per-call | batched | 10 | 124,726 | 4.64x |
| v8go-precompiled | percall | 1 | 29,383 | 1.00x |
| v8go-precompiled | percall | 2 | 55,718 | 1.90x |
| v8go-precompiled | percall | 4 | 96,433 | 3.28x |
| v8go-precompiled | percall | 8 | 127,168 | 4.33x |
| v8go-precompiled | percall | 10 | 131,844 | 4.49x |
| v8go-precompiled | batched | 1 | 33,810 | 1.00x |
| v8go-precompiled | batched | 2 | 65,219 | 1.93x |
| v8go-precompiled | batched | 4 | 108,086 | 3.20x |
| v8go-precompiled | batched | 8 | 141,223 | 4.18x |
| v8go-precompiled | batched | 10 | 136,180 | 4.03x |

#### Row20 through the public Pool, native (Apple M5, aggregate rows/s, median of 10)

| mode | pool | rows/s | speed-up vs pool of 1 |
|---|---|---|---|
| percall | 1 | 2,066 | 1.00x |
| percall | 2 | 3,896 | 1.89x |
| percall | 4 | 5,553 | 2.69x |
| percall | 8 | 8,065 | 3.90x |
| batched | 1 | 2,209 | 1.00x |
| batched | 2 | 4,064 | 1.84x |
| batched | 4 | 5,858 | 2.65x |
| batched | 8 | 8,388 | 3.80x |

**Ratios** (`scripts/bench-ratios.py`, raw file `ratios.md`):

| benchmark | native | baseline-qjs | v8go script-per-call | v8go precompiled | native / v8go script |
|---|---|---|---|---|---|
| GenerateSingle person.firstName | 6.18 us | 34.55 us | 1.10 us | 0.83 us | 5.6x |
| GenerateSingle internet.email | 36.84 us | 110.90 us | 3.21 us | 3.10 us | 11.5x |
| GenerateSingle location.streetAddress | 48.41 us | 139.04 us | 3.65 us | 3.11 us | 13.3x |
| GenerateSingle lorem.sentence | 112.40 us | 206.31 us | 2.05 us | 1.82 us | 54.7x |
| GenerateSingle string.uuid | 49.38 us | 114.51 us | 3.09 us | 2.83 us | 16.0x |
| GenerateSingle number.int | 1.48 us | 20.21 us | 1.14 us | 0.96 us | 1.3x |
| Row20 percall | 478.1 us | 1513.9 us | 39.0 us | 34.0 us | 12.3x |
| Row20 batched | 452.8 us | 835.3 us | 35.4 us | 29.9 us | 12.8x |
| NewEngine | 299 us | 22881 us | 4757 us | 4803 us | 0.1x |

Variance (`benchstat-all.txt`, `benchstat-v8go.txt`): most rows vary by a few percent. The v8go rows vary most,
for example `GenerateSingle/v8go-script-per-call/location.streetAddress` at ±31 % and `number.int` at ±13 %.

Memory (Linux VM, development build, raw file `results/2026-10-17-linux-arm64-vm/bench-rss.txt`): 4.6 MB resident
per native engine and 36.7 MB for a pool of 8; 6.9 MB and 55.0 MB for the baseline.

## 6. Analysis

### 6.1 The call boundary is close to v8go's; the gap is the interpreter

`number.int` costs 1.48 µs per call natively and 1.14 µs with v8go building a script per call: a ratio of 1.3. Native
call overhead is therefore close to v8go's. Methods that do more work in JavaScript show large ratios
(`lorem.sentence` 55×, `string.uuid` 16×). That work runs in QuickJS's bytecode interpreter, while V8 compiles it.
The difference between interpreting and compiling is where the gap comes from. Parallel engines do not close it:
with 10 engines v8go is still 12.6× ahead on Row20 (106,922 against 8,464 rows/s).

### 6.2 Per-method causes, profiled on V8

`lorem.sentence` and `string.uuid` were profiled with Node's CPU profiler on the unminified npm package (300,000
calls each, seed 42, `TZ=UTC`) in an earlier session of this port. That is V8, not the QuickJS module, so it shows the
shape of Faker's code, not the module's costs. The module image is stripped, so a module profile needs a symbolised
build; it was not done. The profile files were not kept, so this section gives no percentages.

- **`lorem.sentence`** calls `words(3)`, which calls `helpers.multiple(() => this.word(), {count: 3})`. Each
  `word()` calls `helpers.arrayElement(G({...e, wordList: definitions.lorem.word}))`, so every word passes through
  the word-list filter and a fresh options object. Most of the Faker-side time was in this per-word call chain,
  little in the filter itself.
- **`string.uuid`** is `'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replaceAll('x', () => number.hex({min: 0, max: 15}))`
  and the same for `y`: 32 callbacks, each calling `number.int` through `number.hex`. These callbacks dominated.

Against the earlier hypotheses: repeated selection is the cost in `lorem.sentence`, and per-character callbacks in
`string.uuid`; regular-expression callbacks and proxy-based locale lookups did not appear among the top frames.
These are statements about Faker's code. How much carries over to the interpreter is a hypothesis until the module
is profiled.

### 6.3 What this means

Where generation dominates an application's runtime, go-fakerjs costs 12.3× more CPU per 20-column row than v8go.
It starts 15.9× faster, needs no cgo or prebuilt library, has no third-party dependency, and reproduces Faker.js
exactly on the tested surface. Whether the trade is worth it depends on how much of the application's time goes to
generating values.

## 7. Negative results

During the development build these were tried and gave no useful gain: `clang -O3` and `wasm-opt -O3`; QuickJS's
switch-based dispatch (`DIRECT_DISPATCH=0`) in place of computed gotos; and Go profile-guided optimisation. `-Oz`
made the generated source smaller but slower. The raw logs of those runs were not kept, so no figures are given;
they are recorded here so the experiments are not repeated blindly.

`wasm-opt --spill-pointers`, which micropython-go needs for its conservative collector, is not applied: QuickJS
counts references and traces from its own roots, and the [module GC stress](#52-robustness) exercises that path.

## 8. Threats to validity

- **One machine for performance.** Every published performance figure comes from one Apple M5. Other CPUs will
  move the ratios.
- **Development build.** The performance figures were measured before v0.1.0. The engine code and snapshot are
  the same, but the measurement has not yet been repeated on the [final build](#54-performance).
- **The UTC shim.** Output matches V8 only under `TZ=UTC`.
- **Node versus v8go.** The oracle is Node's V8 (12.4); v8go v0.34.0 bundles its own V8 build. The two can differ in
  ways the tests do not cover.
- **Workload representativeness.** The 22-path workload is one fixed schema. Column mixes differ.
- **Cross-architecture.** Only linux/arm64 has computed the [golden dump](#53-build-and-reproducibility) so far.
- **[Profiles on V8](#62-per-method-causes-profiled-on-v8), not the module.**
- **One reviewer.** The design and the findings have been reviewed by one person.

## 9. Future work

- **Locale selection.** v0.1.0 ships the en locale only. Supporting others needs Faker's per-locale instances
  exposed in the module, a `WithLocale` option, golden data per locale, and tests.
- **Native Go fast paths** for the hottest methods (`lorem.*`, `string.uuid`), checked by the golden harness so that
  any change in output fails.
- **Build-time bundle patches** that keep the PRNG consumption order but remove interpreter work from the hottest
  Faker code.
- **Pools with derived per-column seeds**, and the determinism trade-off that comes with them: a pool is
  reproducible only while each engine serves one logical stream and none is poisoned.
- **An `Intl` subset**, so that `finance.amount` with `autoFormat` matches V8.
- **Module profiling with symbols**, to replace the [V8 proxy](#62-per-method-causes-profiled-on-v8).
- **Tracking wasm2go to 1.0.** It is pre-1.0 and its output may change; an upgrade is `make snapshot` plus the
  suite.

## 10. Replication

### 10.1 Wall times

Linux VM, final build, idle machine (raw file `results/2026-10-18-linux-arm64-vm/wall-times.txt`, and the race logs
for the race rows). macOS rows come from the raw benchmark files. Other machines will differ.

| step | command | wall time | platform |
|---|---|---|---|
| Toolchain download | `make tools` (empty `tools/`) | 121 s | Linux VM |
| Bundle | `make bundle` (npm cache warm) | under 1 s | Linux VM |
| Module snapshot | `make snapshot` | 10 s | Linux VM |
| Golden data | `make golden-update` | 5 s | Linux VM |
| Golden check | `make golden-verify` | 6 s | Linux VM |
| Tests, root module | `go test ./...` | 100 s | Linux VM |
| Reproducibility | `make snapshot-repro` | 21 s | Linux VM |
| Fuzz smoke | `make test-fuzz FUZZTIME=60s` | 142 s | Linux VM |
| Race, package and suites | `go test -race ./contract ./conformance .` | about 15 minutes (conformance 861 s) | Linux VM |
| Race, full stress | `go test -race ./stress` | 2,649 s | Linux VM |
| Benchmarks, native and baseline | `make bench` | 1,616 s | macOS host (development build) |
| Benchmarks, v8go | `make bench-v8go` | 1,615 s | macOS host (development build) |

### 10.2 macOS (Apple silicon)

```sh
xcode-select --install                 # clang and the CoreFoundation framework
brew install go node                   # Go 1.27.1 or later, Node 22.x
git clone https://github.com/iamramtin/go-fakerjs && cd go-fakerjs
make tools && make doctor              # wasi-sdk, Binaryen, Wizer, wasm2go; writes tools.versions
make snapshot                          # bundle and snapshot; the committed files must not change
make golden-update                     # golden data; make golden-verify checks it
make test                              # vet, unit, conformance, contract, stress, check-versions
make test-stress                       # GOGC=1, gccheckmark and -race (long)
make test-fuzz FUZZTIME=10m
scripts/host/bench-all.sh              # make bench and make bench-v8go, hardware, benchstat, golden hash
```

`bench/v8go_cgo.go` declares `#cgo darwin LDFLAGS: -framework CoreFoundation`, so `make bench-v8go` links without
setting `CGO_LDFLAGS`. Raw output lands in `reports/` and, through `bench-all.sh`, in `results/<date>-apple-m5/`.

### 10.3 Linux

```sh
# Go 1.27.1 or later and Node 22.x from your installer; make, git, curl
git clone https://github.com/iamramtin/go-fakerjs && cd go-fakerjs
make tools && make doctor
make snapshot && make golden-update
make test && make test-stress && make test-fuzz FUZZTIME=10m
make snapshot-repro && make portability
```

`make bench-v8go` on Linux needs v8go's prebuilt V8 dependency module; where it cannot be downloaded the step fails
and nothing else is affected.

### 10.4 Upgrading Faker.js

The [upgrade script](CONTRIBUTING.md#fakerjs-upgrades) automates the upgrade. Its steps were replayed for Faker.js 10.5.0
in a scratch copy on the Linux VM: every step passed, the bundle shrank from 416,501 to 415,672 bytes, the golden
data and the snapshot were regenerated from 10.5.0, and the suite passed (raw file
`results/2026-10-18-linux-arm64-vm/upgrade-drill-10.5.0.txt`). The script's own worktree run needs a commit to
start from, so it has not run yet.

## 11. Raw data index

| path | contents |
|---|---|
| `results/2026-10-17-apple-m5/bench-all.txt`, `bench-v8go.txt` | benchmark output, macOS host, development build |
| `results/2026-10-17-apple-m5/benchstat-all.txt`, `benchstat-v8go.txt`, `ratios.md` | summaries of the two files above |
| `results/2026-10-17-linux-arm64-vm/bench-rss.txt` | resident memory per engine, Linux VM, development build |
| `results/2026-10-18-linux-arm64-vm/` | conformance (native and baseline), bombs, verbose stress, module GC stress, probes, repro, portability, binary size, golden dump, upgrade drill, race logs; Linux VM, final build |
| `golden/testdata/`, `golden/api.json` | golden oracle data |
| `golden/known_divergences.yaml` | the divergence catalogue |
| `reports/` (not committed) | per-run output of tests and benchmarks |

## 12. Credits

Faker.js (MIT), QuickJS-NG (MIT; QuickJS by Fabrice Bellard and Charlie Gordon), wasm2go (MIT), Wizer, Binaryen,
wasi-sdk and esbuild; fastschema/qjs and wazero (baseline); tommie/v8go and V8 (comparison); benchstat. Versions,
[Licences and roles](THIRD_PARTY_NOTICES.md) are listed in full. micropython-go informed the pipeline; no
code was copied from it.

## 13. AI assistance

Designed and tested with the help of Claude Opus 5.5. AI wrote most of the benchmark reports and tests.

- **Models.** The development plan was created with `claude-opus-5-5` based on an initial spike I had done. The v0.1.0 port was started in a session running `claude-haiku-5-5` and reviewed and adjusted with the help of `claude-opus-5-5`.
- **What the AI did.** Test, benchmark design, the Linux VM runs, and the first drafts of these documents.
- **What I did.** Initial spike and code stub, direction and decisions, the benchmark runs on the macOS host, review/audit of the results, and finalisation of the documents.
- **Checking.** Every figure here names a raw file or a command. What could not be produced in the VM (macOS performance on the final build, the darwin golden hash, the CI matrix) is marked as pending.
