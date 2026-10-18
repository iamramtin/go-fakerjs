# Contributing

go-fakerjs is experimental. The most useful contributions are reproductions of a difference from Faker.js, new
test evidence, and reports of a platform where the golden dump hash differs.

## Toolchain

| Tool | Version | Installed by |
|---|---|---|
| Go | 1.27.1 (`go.mod`) | your system |
| Node.js | 22.x (the golden data was generated with 22.23.3) | your system; used by `bundle/` and `golden/` |
| wasi-sdk (clang) | 34 (clang 23.1.0) | `make tools`, into `tools/` (git-ignored) |
| Binaryen (`wasm-opt`) | version_132 | `make tools` |
| Wizer | 11.0.3 | `make tools` |
| wasm2go | v0.4.12 | `make tools` (`go install`) |
| Faker.js | 10.6.0 | npm, from `bundle/package-lock.json` |

Tool pins live in `scripts/pins.env`. The Faker.js version has one source of truth, `tools.versions`; `pins.env`
reads it from there. `make doctor` checks every tool and rewrites `tools.versions` with host-independent
versions only, so the file is the same on every machine with the pinned tools. `make check-versions` fails if
any statement of the Faker.js version (npm manifests and lock files, `golden/api.json`, the golden metadata,
`fakerjs.FakerVersion`, NOTICE, the first CHANGELOG section, and the doc lines listed by
`scripts/check-versions.sh --doc-pins`) disagrees. It also fails on placeholder tokens left in any file.

## Everyday commands

```sh
make bundle          # the Faker.js bundle; conformance tests read it
make test            # vet, unit, conformance, contract and stress tests, check-versions
make test-stress     # GOGC=1, gccheckmark and -race variants
make test-fuzz       # fuzz targets (FUZZTIME=60s for a smoke run)
make check-links     # internal links and anchors in every Markdown file
make bench-smoke     # every benchmark once; checks they compile, not for published numbers
make help            # every target with its description
```

Before a pull request, run `make lint-tools` once, then `make check` (format and lint both modules under every
build variant) and `make vuln`. CI also runs gofmt, `go vet`, staticcheck, govulncheck, and actionlint.

## Regenerating generated files

`internal/wasm/snapshot_en.go`, `internal/wasm/snapshot_en.dat`, `golden/api.json` and `golden/testdata/` are generated
and committed. Do not edit them by hand. `internal/wasm/staticcheck.conf` is hand-written: it turns staticcheck off for the
generated package, whose findings cannot be fixed without breaking reproducibility.

```sh
make tools doctor    # once
make snapshot        # bundle -> clang -> wasm-opt -> Wizer -> wasm2go
make golden-update   # golden data from the npm package, the bundle, and the eval-per-call oracle
make snapshot-repro  # two clean builds from different paths must be byte-identical
```

CI's `regenerate` job rebuilds with the cached pinned toolchain and fails if a committed generated file changes.

## Faker.js upgrades

Upgrades are automated and proven before they are adopted.

### The script

```sh
scripts/faker-upgrade.sh                            # adopt the latest minor or patch release
TARGET=10.5.0 scripts/faker-upgrade.sh --dry-run    # run the whole pipeline for a version; commit nothing
make faker-upgrade                                  # same as the first line
make faker-upgrade FAKER=10.5.0                     # same as the dry run
```

1. It asks npm for the latest `@faker-js/faker`. If that is not newer than the pin, it prints "up to date" and
   exits 0.
2. A new **major** release is never adopted: the script changes nothing and exits 3. Major upgrades are done by
   hand, after reading Faker's migration guide.
3. For a minor or patch release it works in a temporary git worktree, detached at `HEAD`. It updates the pin
   (`tools.versions`, both npm manifests and lock files, `fakerjs.FakerVersion`, NOTICE, and the doc lines from
   `check-versions.sh --doc-pins`), then rebuilds the bundle, regenerates the golden data from the new Faker,
   rebuilds the snapshot (clang, wasm-opt, Wizer, wasm2go), adds a CHANGELOG "Unreleased" entry, and runs
   `check-versions`, `go vet`, the conformance and contract tests, stress with `-short`, a 60 s fuzz smoke test,
   and `snapshot-repro.sh`.
4. On success it commits on a new branch, `faker-upgrade/<version>`, as `chore: Upgrade Faker.js to <version>`. The
   commit body lists the API diff from `golden/api.json` (methods added, removed, and with changed arity) and the
   conformance results. Nothing is pushed.
5. On failure it removes the worktree, creates no branch, and exits 1, naming the failing step and its log file
   (in a temporary directory outside the repository).

A method that the new Faker adds and that throws on every argument variant fails the `golden-update` step by
design: give it variants in `golden/args.json` (below) and run the upgrade again.

### The workflow

`.github/workflows/faker-upgrade.yml` runs the script every Monday and on demand (`workflow_dispatch`; the
`target` input runs a dry run for that version and pushes nothing). On success it pushes the branch, opens a
pull request and enables auto-merge, so the change merges once the required checks pass. On failure, or when a
new major release exists, it opens a tracking issue titled "Faker.js upgrade needs attention" (or comments on the
open one) with the log tail, and attaches the full logs to the run. It pushes nothing in that case.

It needs a secret, **`FAKER_UPGRADE_TOKEN`**: a fine-grained personal access token (or a GitHub App installation
token) for this repository only, with **Contents**, **Pull requests**, and **Issues** set to read and write. The
default `GITHUB_TOKEN` is not enough: pull requests it creates do not trigger other workflows, so CI would never
run on the upgrade pull request and auto-merge would never complete. The workflow's own `permissions` stay at
`contents: read`; every write uses the token.

Auto-merge also needs **Allow auto-merge** enabled in the repository settings and a branch protection rule (or
ruleset) on `main` that requires the CI checks.

## Adding argument variants to `golden/args.json`

`golden/args.json` lists the argument variants for each method; a method without an entry gets one call with no
argument. When golden generation reports that a method throws on every variant, add explicit variants:

```json
"string.alpha": [null, 5, {"length": 5}],
```

If the method throws by design, mark it so the call is still recorded:

```json
"internet.jwt": {"expected_error": "TextEncoder is not defined: absent in a bare V8 context and in QuickJS (no web platform globals)"}
```

Then run `make golden-update` and commit the regenerated `golden/testdata/` and `golden/api.json`.

## Adding a divergence entry

A divergence is a case where the engine and Faker.js on V8 disagree. Each needs a root cause, and category `intl`
when it comes from missing `Intl`. Add an entry to `golden/known_divergences.yaml`:

```yaml
- method: finance.amount
  variant: 1          # index into the method's args.json variants, or "*"
  seed: "*"           # one golden seed, or "*"
  category: intl
  reason: "What differs, the root cause, and what each side prints."
```

The conformance tests enforce the catalogue in both directions: an unlisted divergence fails, and so does an
entry that no longer diverges. Keep each entry to one behaviour; do not widen an entry to `"*"` to make a test
pass. Describe a new category beside the [divergence catalogue](FINDINGS.md#51-conformance) in the findings.

## Benchmarks

Published numbers come only from the macOS host (Apple M5), never from a VM or a CI runner:

```sh
scripts/host/bench-all.sh     # make bench, make bench-v8go, hardware.txt, benchstat, golden hash; about an hour
```

It refuses to run anywhere but darwin/arm64 and writes everything to `results/<date>-apple-m5/`. The tables in
README.md and FINDINGS.md are then regenerated with `scripts/tables.py bench <dir>` and
`scripts/bench-ratios.py`.

## Do not weaken tests

Do not change an expected value, lower a count, or skip a test to make a failure go away. If an expectation is
wrong, fix the golden generator and regenerate, and say why in the commit message.

## Commits

`type: Description`, capitalised, imperative, no trailing period. Types: `feat`, `fix`, `refactor`, `perf`,
`test`, `docs`, `chore`, `ci`. Describe the behaviour that results, not the steps taken.
