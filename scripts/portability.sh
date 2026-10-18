#!/usr/bin/env bash
# D2: cross-builds the library and its tests with CGO disabled for every target, checks the binaries have no
# dynamic library dependencies, runs the host tests, runs the cross-architecture tests when docker is
# available, and compares golden-dump hashes across architectures.
set -uo pipefail
cd -P -- "$(dirname -- "$0")/.." || exit
mkdir -p reports build/portability
out=reports/portability.txt
: > "$out"
say() { echo "$@" | tee -a "$out"; }

targets=(darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64)
go build -o build/portability/dylibs ./scripts/dylibs
fail=0
for t in "${targets[@]}"; do
  os=${t%/*}; arch=${t#*/}; ext=""; [ "$os" = windows ] && ext=".exe"
  bin="build/portability/contract-$os-$arch$ext"
  start=$(date +%s)
  if CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build ./... 2>>"$out" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go test -c -o "$bin" ./contract 2>>"$out"; then
    say "BUILD ok   $t ($(( $(date +%s) - start ))s, test binary $(du -h "$bin" | cut -f1))"
    build/portability/dylibs "$bin" | tee -a "$out"
  else
    say "BUILD FAIL $t"; fail=1
  fi
done

say "--- host tests ($(go env GOOS)/$(go env GOARCH))"
CGO_ENABLED=0 go test -count=1 ./conformance ./contract -run 'TestWorkload22Exact|TestInterleavedSequencesExact|TestSeedSemantics|TestWriteGoldenDump' 2>&1 | tee -a "$out" | tail -5

if command -v docker >/dev/null 2>&1; then
  for plat in linux/amd64 linux/arm64; do
    say "--- docker $plat"
    arch=${plat#*/}
    CGO_ENABLED=0 GOOS=linux GOARCH=$arch go test -c -o "build/portability/conformance-$arch" ./conformance
    docker run --rm --platform "$plat" -v "$PWD":/work -w /work/conformance alpine \
      "/work/build/portability/conformance-$arch" -test.run 'TestWorkload22Exact|TestInterleavedSequencesExact|TestWriteGoldenDump' 2>&1 | tee -a "$out" | tail -3
  done
else
  say "SKIP docker: not installed, so linux/amd64 and linux/arm64 container runs did not happen"
fi

say "--- golden dump hashes by architecture (reports/golden-dump-*.sha256)"
for f in reports/golden-dump-*.sha256; do say "$(cat "$f")  $(basename "$f")"; done
n=$(cat reports/golden-dump-*.sha256 | sort -u | wc -l)
archs=$(for f in reports/golden-dump-*.sha256; do if [ -e "$f" ]; then echo "$f"; fi; done | sed -E 's/.*-(en|all)-([a-z0-9]+)\.sha256/\2/' | sort -u | tr '\n' ' ')
if [ "$n" -eq 1 ]; then say "dumps identical across: $archs"; else say "DUMPS DIFFER ($n distinct) across: $archs"; fail=1; fi
exit $fail
