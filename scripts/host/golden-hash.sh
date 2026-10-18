#!/usr/bin/env bash
# Writes the darwin/arm64 golden dump hash to <results-dir>/golden-dump-darwin-arm64.sha256 and compares it with
# the committed linux/arm64 value. A difference is a cross-platform output difference and exits non-zero.
# Usage: scripts/host/golden-hash.sh [results-dir]    (default: results/<today>-apple-m5)
set -euo pipefail
cd -P -- "$(dirname -- "$0")/../.."
[ "$(uname -s)/$(uname -m)" = Darwin/arm64 ] || { echo "golden-hash.sh: run this on the macOS host (darwin/arm64), not $(uname -s)/$(uname -m)" >&2; exit 1; }
dir=${1:-results/$(date +%F)-apple-m5}
mkdir -p "$dir"
rm -f reports/golden-dump-en-arm64.sha256
go test -count=1 -run '^TestWriteGoldenDump$' ./conformance
cp reports/golden-dump-en-arm64.sha256 "$dir/golden-dump-darwin-arm64.sha256"
ref=$(for f in results/*-linux-arm64-vm/golden-dump-en-arm64.sha256; do if [ -e "$f" ]; then echo "$f"; fi; done | sort | tail -1)
[ -n "$ref" ] || { echo "golden-hash.sh: no committed linux/arm64 golden dump to compare with" >&2; exit 1; }
if cmp -s "$ref" "$dir/golden-dump-darwin-arm64.sha256"; then
  echo "golden-hash.sh: darwin/arm64 equals linux/arm64: $(cat "$ref")"
else
  echo "golden-hash.sh: DIFFERENT. darwin/arm64 $(cat "$dir/golden-dump-darwin-arm64.sha256") vs linux/arm64 $(cat "$ref")" >&2
  exit 1
fi
