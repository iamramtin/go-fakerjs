SHELL := /bin/bash
# Fail a recipe when any command in a pipeline fails (`go test ... | tee` would otherwise exit 0).
.SHELLFLAGS := -eo pipefail -c
export WASI_SDK_PATH ?= $(CURDIR)/tools/wasi-sdk
export PATH := $(CURDIR)/tools/bin:$(CURDIR)/tools/binaryen/bin:$(PATH)

GOTEST ?= go test -count=1 -timeout 30m
FUZZTIME ?= 10m

# Go tool versions are pinned so every machine and CI produce the same result.
GOLANGCI_LINT ?= golangci-lint
GOLANGCI_LINT_VERSION ?= v2.13.2
GOVULNCHECK_VERSION ?= v1.9.0

# Both modules: the root package and the nested bench module.
GO_MODULES := . bench

# Lint variants as module[:build-tags[:GOOS]], so every Go file is linted under at least one of them.
# gcstress_test.go is the exception: it compiles only against the GC-stress module of scripts/gcstress.sh.
LINT_VARIANTS := . .:race .::windows bench bench:baselines bench:v8go

# golangci-lint across LINT_VARIANTS with extra flags $(1). The v8go headers predate C23, the default of newer
# compilers, so cgo is pinned to gnu17.
lint_variants = for v in $(LINT_VARIANTS); do \
	IFS=: read -r m tags goos <<< "$$v"; \
	echo "golangci-lint: module $$m, tags $${tags:-none}, GOOS $${goos:-host}"; \
	(cd $$m && env $${goos:+GOOS=$$goos} CGO_CFLAGS="$${CGO_CFLAGS:--O2 -g} -std=gnu17" \
		$(GOLANGCI_LINT) run --config $(CURDIR)/.golangci.yaml --build-tags "$$tags" $(1)); \
	done

.PHONY: help tools doctor lint-tools clean \
        bundle snapshot golden-update \
        fmt vet lint lint-fix check-lint-version check \
        test test-stress test-fuzz golden-verify snapshot-repro portability check-versions check-links vuln \
        bench bench-v8go bench-baselines bench-smoke \
        faker-upgrade golden

# ============================================================
# Help and setup
# ============================================================

help: ## list the targets with their descriptions
	@grep -E '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | sed -E 's/:.*## /\t/'

tools: ## download the pinned wasm toolchain into ./tools
	./scripts/tools.sh

doctor: ## verify the toolchain and write tools.versions
	./scripts/doctor.sh

lint-tools: ## install golangci-lint at GOLANGCI_LINT_VERSION
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

clean: ## remove the build and reports directories
	rm -rf build reports

# ============================================================
# Build
# ============================================================

bundle: ## build bundle/faker.en.min.js from the pinned npm package
	./bundle/build.sh

snapshot: bundle ## build the pre-initialised en snapshot: clang -> wasm-opt -> Wizer -> wasm2go
	./scripts/snapshot.sh en

golden-update: bundle ## regenerate the golden oracle data (O1/O2/O3)
	cd golden && npm ci --silent && TZ=UTC node enumerate.mjs && TZ=UTC node gen.mjs

# ============================================================
# Format and lint
# ============================================================

fmt: check-lint-version ## format Go code in every module (gofmt, gofumpt, goimports)
	for m in $(GO_MODULES); do (cd $$m && $(GOLANGCI_LINT) fmt --config $(CURDIR)/.golangci.yaml); done

vet: ## run go vet in every module
	for m in $(GO_MODULES); do (cd $$m && go vet ./...); done

lint: check-lint-version ## run golangci-lint over every module and build variant, read-only
	@$(call lint_variants,)

lint-fix: check-lint-version ## run golangci-lint over every module and build variant, applying its fixes
	@$(call lint_variants,--fix)

check-lint-version: ## verify the installed golangci-lint is GOLANGCI_LINT_VERSION
	@have="$$($(GOLANGCI_LINT) version --short 2>/dev/null)"; \
	want="$(GOLANGCI_LINT_VERSION:v%=%)"; \
	if [ "$$have" != "$$want" ]; then \
		echo "golangci-lint $$want required, found '$$have'. Run 'make lint-tools'." >&2; \
		exit 1; \
	fi

check: fmt lint ## format the Go code, then lint it

# ============================================================
# Test and verification
# ============================================================

test: ## vet both modules, run the root module tests, and run the version check
	go vet ./...
	cd bench && go vet ./...
	$(GOTEST) ./...
	./scripts/check-versions.sh

test-stress: ## run the robustness tests under GOGC=1, gccheckmark and -race
	GOGC=1 $(GOTEST) ./stress ./contract ./conformance
	GOGC=1 GODEBUG=gccheckmark=1 $(GOTEST) -run 'TestGCStress|TestIsolation' ./stress
	$(GOTEST) -race ./contract ./conformance .
	$(GOTEST) -race -short ./stress

test-fuzz: ## fuzz FuzzGenerate for FUZZTIME (default 10m) and FuzzSnapshotArgs for 1m
	$(GOTEST) -run '^$$' -fuzz FuzzGenerate -fuzztime $(FUZZTIME) ./stress
	$(GOTEST) -run '^$$' -fuzz FuzzSnapshotArgs -fuzztime 1m .

golden-verify: bundle ## regenerate the golden data in memory and require it to equal golden/testdata, with O1 == O2
	cd golden && npm ci --silent && TZ=UTC node gen.mjs --verify

snapshot-repro: ## build the snapshot twice, the second time from another path, and require byte-identical output
	./scripts/snapshot-repro.sh en

portability: ## cross-build five platforms, check linkage, run host tests, and compare golden dumps
	./scripts/portability.sh

check-versions: ## verify version statements match the pins and no placeholder tokens remain
	./scripts/check-versions.sh

check-links: ## verify internal links and anchors in every Markdown file resolve
	python3 scripts/check-links.py

vuln: ## scan the dependencies and standard library of every module with govulncheck
	for m in $(GO_MODULES); do (cd $$m && go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...); done

# ============================================================
# Benchmarks
# ============================================================

bench: ## run the native and baseline benchmarks at -count=10, raw output in reports/bench-all.txt
	mkdir -p reports
	cd bench && go test -run '^$$' -bench . -benchmem -count=10 -timeout 4h ./... | tee ../reports/bench-all.txt

bench-v8go: ## run the same workload on tommie/v8go (cgo; needs its prebuilt V8), raw output in reports/bench-v8go.txt
	mkdir -p reports
	cd bench && go test -tags v8go -run '^$$' -bench 'Row20|GenerateSingle|NewEngine' -benchmem -count=10 -timeout 3h ./ | tee ../reports/bench-v8go.txt

bench-baselines: ## measure conformance of the wazero + QuickJS baseline; exits non-zero by design (no Date shim), counts in reports/
	cd bench && go test -tags baselines -v -timeout 40m ./...

bench-smoke: ## compile and run every benchmark once (-benchtime=1x); not for published numbers
	cd bench && go test -run '^$$' -bench . -benchtime=1x -timeout 30m ./...

# ============================================================
# Release
# ============================================================

# Retired names. They stay as rules so an old command fails loudly instead of doing nothing.
golden:
	@echo "renamed: use make golden-update or make snapshot" >&2; exit 1

faker-upgrade: ## upgrade Faker.js to the latest minor or patch release, or dry-run FAKER=<version> (see CONTRIBUTING.md)
	$(if $(FAKER),TARGET=$(FAKER) ./scripts/faker-upgrade.sh --dry-run,./scripts/faker-upgrade.sh)
