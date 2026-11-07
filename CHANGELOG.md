# Changelog

All notable changes are recorded here. Before v1.0.0, any minor release may change the API.

## Unreleased

Still Faker.js 10.6.0, en locale. The API and the generated output are unchanged.

### Added

- The project icon, in light and dark variants, and the social card, in `docs/brand/`.
- `make cover` and a CI `coverage` job that uploads the report to Codecov, with tests for the WASI shim, `Error`
  messages, and `Pool.Size`. Coverage of the package and the WASI shim across every suite is 90.7%.

### Changed

- The README is rewritten around a quick start for a new module, every result shape and error class from real
  runs, a coverage table of all 27 modules, and determinism and compatibility. Detail moved to
  `docs/how-it-works.md`, `docs/determinism.md`, and `docs/performance.md`, with every figure kept.
- CI installs Node 22.23.3, the version the golden data records, instead of the latest Node 22 release, which
  had made the `regenerate` check fail on the recorded version alone.

### Fixed

- The compatibility statement names the oracle exactly: Faker.js on V8 without the web-platform globals Node
  adds. `internet.jwt` and base64 `image.dataUri` throw in go-fakerjs and in the oracle, and return values in a
  stock Node process. The v0.1.0 limitation below is corrected to say so.
- FINDINGS no longer describes the final-build benchmarks, the four-platform golden dump comparison, and the weekly
  upgrade workflow as pending.

## v0.1.0 (2026-10-20)

First public release, experimental. Runs Faker.js 10.6.0, en locale.

### Added

- `Engine` (`NewEngine`, `Generate`, `GenerateN`, `Close`, `Poisoned`): Faker.js from pure Go, with no cgo, no
  Wasm runtime and no third-party dependency.
- `Pool` (`NewPool`, `Get`, `Do`, `Close`) and `DeriveSeed` for parallel use with per-engine seeds.
- Options: `WithRefDate`, `WithTimeout`, `WithMaxArgBytes`, `WithClock`.
- Errors: `Error` with classes (`ClassUnknownMethod` and others) and sentinels (`ErrDeadline`, `ErrPoisoned`,
  `ErrOutOfMemory`, `ErrResultTooLarge`, `ErrArgTooLarge`, `ErrUnknownMethod`, `ErrClosed`).
- `FakerVersion`, the embedded Faker.js release.
- The generated en snapshot, committed so the module builds without the toolchain.
- Golden conformance data for 262 methods, a divergence catalogue, and three oracles (npm package, bundle,
  eval-per-call), with the tests that use them.
- Robustness tests: resource bombs, deadlines, GC stress, isolation, and fuzz targets.
- Build and maintenance: `make tools`, `doctor`, `bundle`, `snapshot`, `golden-update`, `golden-verify`,
  `snapshot-repro`, `portability`, `check-versions`, `check-links`, and `faker-upgrade`
  (`scripts/faker-upgrade.sh`) with a weekly upgrade workflow.
- Lint and audit: `make fmt`, `vet`, `lint`, `lint-fix`, `check`, and `vuln`, with golangci-lint pinned by
  `make lint-tools`.
- CI: lint, govulncheck, tests for every module, race, fuzz smoke, a cross-platform golden dump comparison, a
  regeneration check, version and link checks, and an on-demand benchmark job.

### Not yet supported

- Locales other than en. Locale selection is [future work](FINDINGS.md#9-future-work).
- `Intl`: `finance.amount` with `autoFormat` differs from V8.
- Time zones other than UTC in `Date` text.

### Known limitations

- Output matches Faker.js on V8 for the tested surface only, under `TZ=UTC`, and in a V8 environment without
  Node's web-platform globals: `internet.jwt` and base64 `image.dataUri` throw, where Faker.js on stock Node
  returns values.
- Per call, the engine is 1.3× to 55× slower than v8go depending on the method, and 12.1× slower on a
  20-column row (macOS host, [performance figures](README.md#performance)).
