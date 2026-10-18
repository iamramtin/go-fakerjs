# Changelog

All notable changes are recorded here. Before v1.0.0, any minor release may change the API.

## v0.1.0 (not yet tagged)

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

- Output matches Faker.js on V8 for the tested surface only, under `TZ=UTC`.
- Per call, the engine is 1.3× to 55× slower than v8go depending on the method, and 12.3× slower on a
  20-column row (macOS host, [development build](README.md#performance)).
