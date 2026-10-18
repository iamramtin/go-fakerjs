# Third-party notices

go-fakerjs is MIT-licensed (see [LICENSE](LICENSE)). It contains, or is produced with, the components below.
Each licence was read from the file named in the table, at the version given, unless the table says otherwise.

## What the committed generated files contain

The committed snapshot is `internal/wasm/snapshot_en.go` and `internal/wasm/snapshot_en.dat`: a WebAssembly image
translated by wasm2go. The image contains:

- **Faker.js 10.6.0** with its en locale data, bundled by esbuild: MIT.
- **QuickJS-NG v0.17.0** (MIT), a fork of QuickJS by Fabrice Bellard and Charlie Gordon.
- **The C runtime linked into the module** from wasi-libc and the LLVM compiler runtime, built with wasi-sdk 34:
  wasi-libc is multi-licensed under Apache-2.0 with LLVM exceptions, Apache-2.0, and MIT (its own LICENSE
  names the component families, including CC0 for dlmalloc and MIT for musl parts); the compiler runtime is
  Apache-2.0 with LLVM exceptions. The module is stripped, so the exact objects linked were not enumerated.
- **esbuild's runtime helpers**, which appear in the minified bundle: MIT.
- **The go-fakerjs harness** (`wasm/harness.c`) and the WASI shim (`internal/wasi`): MIT, this project.

wasm2go writes the Go code; its output is not a copy of its own source, and it is licensed under MIT (below).
So the Go files and data files are covered by MIT (project code and the Faker.js and QuickJS-NG notices above)
and by the wasi-libc and LLVM licences for the runtime objects they contain. Keep these notices with any
redistribution of the snapshots.

## Bundled into the generated code

| Component | Source | Version / revision | Licence | Copyright line (from the licence file) | Role |
|---|---|---|---|---|---|
| Faker.js (`@faker-js/faker`) | https://github.com/faker-js/faker | 10.6.0 (npm tarball) | MIT | `Faker - Copyright (c) 2022-2025` | bundled into the module |
| QuickJS-NG | https://github.com/quickjs-ng/quickjs | v0.17.0, commit `6d46d07d04041b40f4f49eaa7fdebe44c314c699` | MIT | `Copyright (c) 2017-2026 Fabrice Bellard`; `Copyright (c) 2017-2024 Charlie Gordon`; `Copyright (c) 2023-2026 Ben Noordhuis` | bundled into the module |
| wasi-libc | https://github.com/WebAssembly/wasi-libc | revision used by wasi-sdk 34 not identified; licence text read from `main` | Apache-2.0 with LLVM exceptions, Apache-2.0, MIT (see LICENSE in that repository) | per component | linked into the module |
| LLVM compiler runtime | https://github.com/llvm/llvm-project | as shipped in clang 23.1.0 (wasi-sdk 34) | Apache-2.0 with LLVM exceptions (the LLVM licence header in the shipped clang headers) | per file | linked into the module |
| esbuild helpers | https://github.com/evanw/esbuild | 0.25.10 (npm) | MIT | `Copyright (c) 2020 Evan Wallace` | appear in the bundle |

## Build-time tools (downloaded by `make tools`; not redistributed)

| Component | Source | Version | Licence (file) | Copyright line | Role |
|---|---|---|---|---|---|
| wasm2go | https://github.com/ncruces/wasm2go | v0.4.12 | MIT (`LICENSE`) | `Copyright (c) 2026 Nuno Cruces` | translates the Wasm image to Go |
| Wizer | https://github.com/bytecodealliance/wizer | 11.0.3 | Apache-2.0 (`LICENSE`; the file has no copyright line) | none in the licence file | pre-initialises the snapshot |
| Binaryen (`wasm-opt`) | https://github.com/WebAssembly/binaryen | version_132 | Apache-2.0 (`LICENSE`; no copyright line) | none in the licence file | optimises the Wasm |
| wasi-sdk (clang, sysroot) | https://github.com/WebAssembly/wasi-sdk | 34 (clang 23.1.0) | Apache-2.0 (`LICENSE`) | none in the licence file | compiles the module |
| esbuild | https://github.com/evanw/esbuild | 0.25.10 | MIT (`LICENSE.md`) | `Copyright (c) 2020 Evan Wallace` | bundles Faker.js |
| Node.js | https://github.com/nodejs/node | v22.23.3 | Node.js licence (`LICENSE`: MIT-style grant plus the notices for bundled dependencies) | `Copyright Node.js contributors` | test-only oracle and golden generator |

## Test-only oracles and benchmark baselines

Not part of the library's import graph; used by `golden/`, `bench/`, and the baseline tests.

| Component | Source | Version | Licence (file) | Copyright line | Role |
|---|---|---|---|---|---|
| fastschema/qjs | https://github.com/fastschema/qjs | v0.0.6 | MIT (`LICENSE`) | `Copyright (c) 2025 Nguyen Ngoc Phuong and Contributors` | benchmark baseline (QuickJS on wazero) |
| wazero | https://github.com/tetratelabs/wazero | v1.9.0 | Apache-2.0 (`LICENSE`; no copyright line) | none in the licence file | runtime for the baseline |
| tommie/v8go | https://github.com/tommie/v8go | v0.34.0 | BSD-3-Clause (`LICENSE`) | `Copyright (c) 2019 Roger Chapman and the v8go contributors` | benchmark baseline, built with `-tags v8go` only |
| V8 | https://github.com/v8/v8 | licence read from `main`; the revision inside v8go v0.34.0 was not fetched (its dependency modules are not reachable from the build environment) | BSD-3-Clause (V8 `LICENSE`) | none in the licence file | runtime behind v8go |
| benchstat (`golang.org/x/perf`) | https://pkg.go.dev/golang.org/x/perf/cmd/benchstat | pseudo-version `v0.0.0-20261009192801-be2c69fb417e` | BSD-3-Clause (`LICENSE`) | `Copyright 2009 The Go Authors` | summarises benchmark results |
| staticcheck (`honnef.co/go/tools`) | https://honnef.co/go/tools | v0.8.2 | MIT (`LICENSE` in the module) | `Copyright (c) 2016 Dominik Honnef` | CI lint only |

## Inspiration

| Component | Licence | Notes |
|---|---|---|
| micropython-go | Apache-2.0, `Copyright 2026 Gregory Furman` | Its approach informed the pipeline. |

## Verification notes

- Licence texts above were read from the module cache, the npm tarballs in `bundle/node_modules`, the pinned
  QuickJS-NG source tree, or the upstream repository at the tag in the table. Where a table entry says "no
  copyright line", the licence file itself has none, and this file does not supply one.
- The copyright line for `micropython-go` comes from its licence appendix (`Copyright 2026 Gregory Furman`). Its upstream URL is not recorded in this repository, so none is given.
