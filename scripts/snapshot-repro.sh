#!/usr/bin/env bash
# D1: builds the wasm pipeline twice from clean state - the second time from a copy of the repository at a
# different absolute path - and requires identical sha256 for the optimised wasm, the wizened wasm, the
# generated Go and its data file.
# Usage: scripts/snapshot-repro.sh [en]
set -euo pipefail
cd -P -- "$(dirname -- "$0")/.."
root=$PWD
variants=("${@:-en}")
mkdir -p reports
log="$root/reports/repro.txt"
: > "$log"

gendir() { # variant -> directory holding the generated Go and data file
  case $1 in en) echo internal/wasm ;; esac
}

hashes() { # dir variant
  local d=$1 v=$2 g
  g=$(gendir "$v")
  for f in "$d/build/snapshot_$v.opt.wasm" "$d/build/snapshot_$v.wizened.wasm" "$d/$g/snapshot_$v.go" "$d/$g/snapshot_$v.dat"; do
    printf '%s  %s\n' "$(sha256sum "$f" | cut -d' ' -f1)" "$(basename "$f")"
  done
}

other=$(mktemp -d)
trap 'rm -rf "$other"' EXIT
mkdir -p "$other/b" && cd "$other/b"
cp -r "$root/wasm" "$root/scripts" "$root/tools.versions" "$other/b/"
mkdir -p bundle build
cp "$root"/bundle/faker.*.min.js bundle/
ln -s "$root/tools" tools

status=0
for v in "${variants[@]}"; do
  rm -rf "$root/build/snapshot_$v".* "$root/$(gendir "$v")/snapshot_$v".*
  (cd "$root" && ./scripts/snapshot.sh "$v" >/dev/null)
  hashes "$root" "$v" > "$other/$v.1"
  ./scripts/snapshot.sh "$v" >/dev/null
  hashes "$other/b" "$v" > "$other/$v.2"
  if diff -u "$other/$v.1" "$other/$v.2" > /dev/null; then
    echo "REPRODUCIBLE [$v]" | tee -a "$log"; cat "$other/$v.1" | tee -a "$log"
  else
    echo "NOT REPRODUCIBLE [$v]" | tee -a "$log"; diff -u "$other/$v.1" "$other/$v.2" | tee -a "$log"; status=1
  fi
done
exit $status
