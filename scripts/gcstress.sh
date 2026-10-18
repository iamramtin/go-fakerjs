#!/usr/bin/env bash
# Module-side GC stress: builds the module with QuickJS's FORCE_GC_AT_MALLOC behaviour (a full cycle collection
# before every allocation, switched on only after Wizer's init), in a scratch copy of the repository, and
# replays golden calls against it. Evidence that no JSValue is dropped or double-freed by the harness, which
# is why wasm-opt --spill-pointers is not needed.
set -euo pipefail
cd -P -- "$(dirname -- "$0")/.."
root=$PWD
. scripts/pins.env
copy=$(mktemp -d)
trap 'rm -rf "$copy"' EXIT
tar -C "$root" --exclude=./build --exclude=./reports --exclude='*/node_modules' --exclude=./tools --exclude='./.*' --exclude='internal/wasm/snapshot_*' -cf - . | tar -C "$copy" -xf -
ln -s "$root/tools" "$copy/tools"
cp "$root"/bundle/faker.*.min.js "$copy/bundle/"
mkdir -p "$copy/build" "$copy/reports"
cd "$copy"
./scripts/fetch-quickjs.sh
grep -c 'force_gc = true;' build/quickjs-ng/quickjs.c | grep -q '^1$' || { echo "quickjs.c layout changed: expected exactly one 'force_gc = true;'" >&2; exit 1; }
sed -i 's/force_gc = true;/force_gc = g_gc_stress;/' build/quickjs-ng/quickjs.c
echo 'extern int g_gc_stress;' > build/gcstress_decl.h
EXTRA_CFLAGS="-DFORCE_GC_AT_MALLOC -DGC_STRESS -include $copy/build/gcstress_decl.h" ./scripts/snapshot.sh en >/dev/null
GCSTRESS_CALLS=${GCSTRESS_CALLS:-40} go test -count=1 -tags gcstress -run TestSnapshotGCStress -v -timeout 60m . | tee "$root/reports/gcstress.txt"
