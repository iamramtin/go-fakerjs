#!/usr/bin/env bash
# Builds the wasm32 WASI reactor: QuickJS-NG + harness + embedded Faker bundle.
# Usage: wasm/build.sh en <out.wasm>
set -euo pipefail
cd -P -- "$(dirname -- "$0")/.."
variant=${1:?variant en}
out=${2:?output .wasm}
export WASI_SDK_PATH="${WASI_SDK_PATH:-$PWD/tools/wasi-sdk}"
./scripts/fetch-quickjs.sh
QJS=build/quickjs-ng
CLANG="$WASI_SDK_PATH/bin/clang"
bundle="$PWD/bundle/faker.$variant.min.js"
[ -f "$bundle" ] || { echo "missing $bundle: run 'make bundle'" >&2; exit 1; }
mkdir -p "$(dirname "$out")"

# Features are pinned explicitly (not left to the compiler default) so the output is the same on every
# host and Wizer, which does not support reference types, sees none.
FEATURES=(-mbulk-memory -msign-ext -mnontrapping-fptoint -mmultivalue -mmutable-globals -mno-reference-types -mno-simd128 -mno-atomics)
# OPT and EXTRA_CFLAGS are whitespace-separated flag lists.
read -ra opt <<< "${OPT:--O2}"
read -ra extra <<< "${EXTRA_CFLAGS:-}"

"$CLANG" --target=wasm32-wasip1 -std=gnu23 "${opt[@]}" -DNDEBUG "${FEATURES[@]}" \
  -D_GNU_SOURCE -D_WASI_EMULATED_PROCESS_CLOCKS -D_WASI_EMULATED_SIGNAL \
  ${extra[@]+"${extra[@]}"} \
  -Wno-unused-function -Wno-implicit-fallthrough -Wno-sign-compare -Wno-unused-parameter \
  -funsigned-char -ffile-prefix-map="$PWD"=. -fno-ident \
  -I"$QJS" -Iwasm -DBUNDLE_FILE="\"$bundle\"" \
  wasm/harness.c "$QJS/libregexp.c" "$QJS/libunicode.c" "$QJS/dtoa.c" \
  -mexec-model=reactor -lwasi-emulated-process-clocks -lwasi-emulated-signal \
  -Wl,--no-entry -Wl,--wrap=gettimeofday -Wl,--wrap=clock_gettime \
  -Wl,--export=malloc -Wl,--export=free \
  -Wl,-z,stack-size=2097152 -Wl,--max-memory=268435456 \
  -Wl,--strip-debug \
  -o "$out"
ls -l "$out"
