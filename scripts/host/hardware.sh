#!/usr/bin/env bash
# Records the measuring Mac's hardware, OS and Go version in <results-dir>/hardware.txt. No hostname is recorded.
# Usage: scripts/host/hardware.sh [results-dir]    (default: results/<today>-apple-m5)
set -euo pipefail
cd -P -- "$(dirname -- "$0")/../.."
[ "$(uname -s)/$(uname -m)" = Darwin/arm64 ] || { echo "hardware.sh: run this on the macOS host (darwin/arm64), not $(uname -s)/$(uname -m)" >&2; exit 1; }
dir=${1:-results/$(date +%F)-apple-m5}
mkdir -p "$dir"
{ sysctl -n hw.model machdep.cpu.brand_string hw.ncpu hw.memsize; sw_vers; go version; } > "$dir/hardware.txt"
echo "hardware.sh: wrote $dir/hardware.txt"
