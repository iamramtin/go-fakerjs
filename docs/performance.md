# Performance

Throughput is the cost of running Faker.js in QuickJS-NG. QuickJS-NG interprets Faker's JavaScript, whereas V8 compiles it, so each call is slower than on V8. Start-up is faster, because Wizer evaluated the Faker.js bundle when the snapshot was built.

Every figure on this page comes from an Apple M5 (darwin/arm64, 10 cores), with medians of 10 runs, measured on 2026-10-20 on the final build with the same pinned Faker.js and toolchain as v0.1.0. The raw files are in [`results/2026-10-20-apple-m5/`](../results/README.md), and the [findings](../FINDINGS.md#54-performance) have every table, including `GenerateN` per batch size and `Pool` scaling.

## Comparison

| | v8go | go-fakerjs | wazero with QuickJS |
| --- | --- | --- | --- |
| How the JavaScript runs | V8 with its JIT, through cgo | QuickJS-NG bytecode interpreter, compiled to Wasm and translated to Go | QuickJS-NG compiled to Wasm, run by the wazero runtime |
| Engine start-up, first value included | 4.75 ms | 0.31 ms | 23.4 ms |
| Average time per fake value (20-column row, one call per value) | 1.97 µs | 23.8 µs | 73.8 µs |
| Rows/s, 20 fake columns, 1 engine | 25,339 | 2,100 | 677 |
| Rows/s, 20 fake columns, 10 engines | 111,128 | 9,652 | 727 |
| Output identical to Faker.js | V8 itself; not run through the golden suite here | yes on the tested surface, 5 `Intl` divergences | 2,135 of 2,240 per-method cases exact; dates differ |
| cgo and platform limits | cgo; prebuilt V8 for linux and darwin (amd64, arm64) and android | none: pure Go | none: pure Go |

v8go is tommie/v8go v0.34.0, building a script per call. The wazero baseline's conformance was measured on the Linux VM.

## Per method

One `Generate` call, ns/op:

| Method | go-fakerjs | v8go, script per call | Ratio |
| --- | ---: | ---: | ---: |
| `number.int` | 1,474 | 1,154 | 1.3× |
| `person.firstName` | 6,138 | 1,071 | 5.7× |
| `internet.email` | 36,423 | 3,402 | 10.7× |
| `location.streetAddress` | 47,754 | 3,837 | 12.4× |
| `string.uuid` | 48,984 | 3,067 | 16.0× |
| `lorem.sentence` | 112,520 | 2,052 | 54.8× |

The call boundary costs about the same as v8go's: `number.int`, which does little JavaScript work, is 1.3× slower. Methods that run more JavaScript show the interpreter's cost, and parallel engines don't close the gap: with 10 engines, v8go is still 11.5× ahead on the 20-column row ([analysis](../FINDINGS.md#61-the-call-boundary-is-close-to-v8gos-the-gap-is-the-interpreter)).

## Memory and binary size

- **Resident memory.** 4.6 MB per engine, and 36.7 MB for a pool of 8 (Linux VM, development build).
- **Binary size.** The snapshot adds 10.2 MB to a stripped binary.
- **Build cost.** A cold build takes 8.9 s and peaks at about 1.9 GB of RSS (Linux VM, [sizes](../FINDINGS.md#53-build-and-reproducibility)).

## Re-running

```sh
scripts/host/bench-all.sh     # make bench and make bench-v8go, hardware, benchstat, golden hash; about an hour
```

The script runs only on darwin/arm64 and writes to `results/<date>-apple-m5/` ([benchmarks](../CONTRIBUTING.md#benchmarks)). CI runner numbers aren't comparable with the macOS host's.
