#!/usr/bin/env bash
# Full wasm pipeline for one variant:
#   clang (wasm/build.sh) -> wasm-opt -O2 -> wizer (Faker evaluated into the snapshot) -> wasm2go -> Go
# Usage: scripts/snapshot.sh en [outdir=build]
set -euo pipefail
cd -P -- "$(dirname -- "$0")/.."
variant=${1:?variant en}
work=${2:-build}
export PATH="$PWD/tools/bin:$PWD/tools/binaryen/bin:$PATH"
# Only the en snapshot is built; locale selection is future work (FINDINGS.md section 9).
case $variant in
  en) gendir=internal/wasm ;;
  *) echo "unknown variant $variant" >&2; exit 1 ;;
esac
mkdir -p "$work" "$gendir"

raw="$work/snapshot_$variant.raw.wasm"
opt="$work/snapshot_$variant.opt.wasm"
wiz="$work/snapshot_$variant.wizened.wasm"

# Feature set is whatever wasm/build.sh pins; spelled out so wasm-opt validates the same set.
FEATURES=(--enable-mutable-globals --enable-nontrapping-float-to-int --enable-sign-ext --enable-multivalue --enable-bulk-memory --enable-extended-const --enable-call-indirect-overlong)
read -ra wasmopt <<< "${WASMOPT:--O2}"

./wasm/build.sh "$variant" "$raw" >/dev/null
wasm-opt "${FEATURES[@]}" "${wasmopt[@]}" "$raw" -o "$opt"
# --wasm-bulk-memory/multi-value: Wizer does not enable them by default. Reference types are unsupported by
# Wizer and are disabled in the module build. No --allow-wasi: init must not touch the host (see wasm/README.md).
wizer "$opt" -o "$wiz" --init-func wizer-initialize --wasm-bulk-memory=true --wasm-multi-value=true

wasm2go -pkg wasm -embed -unsafe -o "$gendir/snapshot_$variant.go" "$wiz"
gofmt -w "$gendir/snapshot_$variant.go"
printf '%s raw=%s opt=%s wizened=%s go=%s dat=%s\n' "$variant" \
  "$(wc -c <"$raw")" "$(wc -c <"$opt")" "$(wc -c <"$wiz")" \
  "$(wc -c <"$gendir/snapshot_$variant.go")" "$(wc -c <"$gendir/snapshot_$variant.dat")"
