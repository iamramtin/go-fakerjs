# Results

Raw output behind the figures in [README.md](../README.md) and [FINDINGS.md](../FINDINGS.md). Each folder is named
after the date and the machine. Files are copied unchanged from the run, except that nothing in them may name a
local path, host or user (none did).

## `2026-10-17-apple-m5/`: macOS host, darwin/arm64, Apple M5

**To be refreshed on the final build.** These runs used the development build that preceded v0.1.0, with the same
pinned Faker.js 10.6.0 and toolchain; the engine code and the snapshot are the same as v0.1.0's apart from comments,
a constant and a build tag. `scripts/host/bench-all.sh` repeats them on the final build into a new dated folder.

| File | What it is | How it was produced |
|---|---|---|
| `bench-all.txt` | Native engine and the wazero + QuickJS baseline: `GenerateSingle`, `GenerateN`, `Row20`, `Row20Parallel`, `Row20Pool`, `NewEngine`, `NewFromSnapshot`; `-count=10` | `make bench` |
| `bench-v8go.txt` | The same workload on tommie/v8go v0.34.0 (script built per call, and precompiled), with native and baseline rows from the same run | `make bench-v8go` |
| `benchstat-all.txt`, `benchstat-v8go.txt` | benchstat summaries (mean and ± variance over 10 runs) | `benchstat <file>` (`golang.org/x/perf`) |
| [`ratios.md`](2026-10-17-apple-m5/ratios.md) | Medians and the native-to-v8go ratios quoted in the docs | `scripts/bench-ratios.py bench-all.txt bench-v8go.txt` |

The hardware fields of this machine were not recorded at the time. `scripts/host/hardware.sh` writes them, on the
Mac, to the folder of the run it belongs to.

## `2026-10-17-linux-arm64-vm/`: Linux VM, linux/arm64, 4 vCPU, development build

| File | What it is | How it was produced |
|---|---|---|
| `bench-rss.txt` | Resident memory per engine and for a pool of 8, native and baseline | `cd bench && go test -run TestResidentMemoryPerEngine -v ./` |

## `2026-10-18-linux-arm64-vm/`: Linux VM, linux/arm64, 4 vCPU, final build

| File | What it is | How it was produced |
|---|---|---|
| `conformance-native-en.json` | Conformance counts by category and module for the en snapshot | `go test ./conformance` |
| `conformance-baseline-qjs.json` | The same for the wazero + QuickJS baseline | `make bench-baselines` |
| `bombs.json` | Resource bombs: outcome, time, peak RSS, engine usable afterwards | `go test -v ./stress` |
| `stress-verbose.txt` | Verbose stress run: GC stress, leak, churn, deadlines, isolation, bombs | `go test -count=1 -v ./stress` |
| `guest-gcstress.txt` | Guest built with a full QuickJS collection before every allocation, replaying golden calls | `scripts/gcstress.sh` |
| `probes.txt` | QuickJS against V8 on 73 probes | `node golden/probes.mjs`, then `go test -tags baselines -run TestQJSProbes -v` in `bench/` |
| `repro.txt` | SHA-256 of the build artefacts from two clean builds at different paths | `make repro` |
| `portability.txt` | Cross-compiled test binaries for five platforms and their dynamic libraries | `make portability` |
| `binsize.txt` | Stripped binary size, cold build time and peak RSS | `scripts/binsize.sh` |
| `golden-dump-en-arm64.sha256` | The golden dump hash for linux/arm64 | `go test -run TestWriteGoldenDump ./conformance` |
| `upgrade-drill-10.5.0.txt` | The upgrade pipeline's steps replayed for Faker.js 10.5.0 | the steps of `scripts/faker-upgrade.sh`, in a scratch copy |
| `race-root-contract-conformance.txt` | Race detector on the package, contract and conformance suites | `go test -race -count=1 ./contract ./conformance .` |
| `race-stress-full.txt` | Race detector on the stress suite at full budgets | `go test -race -count=1 ./stress` |
| `fuzz-race-10m.txt` | `FuzzGenerate` for 10 minutes under the race detector | `go test -race -run '^$' -fuzz FuzzGenerate -fuzztime 10m ./stress` |
| `fuzz-smoke-60s.txt` | The CI fuzz smoke test | `make test-fuzz FUZZTIME=60s` |
| `wall-times.txt` | Wall times of the replication steps on an idle VM | the commands it lists |

## Re-running

On the Mac (published numbers):

```sh
scripts/host/bench-all.sh     # about an hour; writes results/<today>-apple-m5/
```

Anywhere (tests and robustness): the commands in the tables above, after `make bundle`. CI's `bench` job runs
`make bench` on demand and uploads the raw output; CI runner numbers are not comparable with the macOS host's.
