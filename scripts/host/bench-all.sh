#!/usr/bin/env bash
# Runs the published benchmarks on the macOS host and writes everything to results/<today>-apple-m5/:
# raw output (make bench, make bench-v8go; -count=10), hardware.txt, benchstat summaries, ratios.md and the
# darwin/arm64 golden dump hash. Expect about an hour. Stops at the first failing step.
# Usage: scripts/host/bench-all.sh [results-dir]    (default: results/<today>-apple-m5)
set -euo pipefail
cd -P -- "$(dirname -- "$0")/../.."
[ "$(uname -s)/$(uname -m)" = Darwin/arm64 ] || { echo "bench-all.sh: refusing to run on $(uname -s)/$(uname -m); published numbers come from the darwin/arm64 host only" >&2; exit 1; }

BENCHSTAT=golang.org/x/perf/cmd/benchstat@v0.0.0-20261009192801-be2c69fb417e
dir=${1:-results/$(date +%F)-apple-m5}
start=$(date +%s)
step() { # name, command...
  local name=$1; shift
  echo "==> $name"
  if ! "$@"; then echo "bench-all.sh: FAILED at step: $name" >&2; exit 1; fi
}
mkdir -p "$dir"
step "bundle" make bundle
step "make bench (native and baseline, -count=10)" make bench
step "copy bench-all.txt" cp reports/bench-all.txt "$dir/bench-all.txt"
step "make bench-v8go (-count=10)" make bench-v8go
step "copy bench-v8go.txt" cp reports/bench-v8go.txt "$dir/bench-v8go.txt"
step "hardware" scripts/host/hardware.sh "$dir"
step "benchstat bench-all" sh -c "go run $BENCHSTAT '$dir/bench-all.txt' > '$dir/benchstat-all.txt'"
step "benchstat bench-v8go" sh -c "go run $BENCHSTAT '$dir/bench-v8go.txt' > '$dir/benchstat-v8go.txt'"
step "ratios" sh -c "python3 scripts/bench-ratios.py '$dir/bench-all.txt' '$dir/bench-v8go.txt' > '$dir/ratios.md'"
step "golden hash" scripts/host/golden-hash.sh "$dir"

secs=$(( $(date +%s) - start ))
echo
echo "bench-all.sh: done in $((secs / 60))m$((secs % 60))s. Results: $dir"
echo "To hand the results back for table regeneration:"
echo "  1. git add $dir && git commit -m 'perf: Record benchmarks on the final build'"
echo "  2. In the next session, ask for the tables to be regenerated from $dir:"
echo "       python3 scripts/tables.py bench $dir"
echo "     The README performance table, FINDINGS section 5.4 and results/README.md are then updated from it."
