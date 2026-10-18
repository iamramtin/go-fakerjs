#!/usr/bin/env bash
# Verifies the toolchain, prints versions, and writes tools.versions. Only host-independent versions are
# recorded, so the file is the same on every machine with the pinned tools; the faker line is kept as is.
set -uo pipefail
cd -P -- "$(dirname -- "$0")/.." || exit
. scripts/pins.env

export WASI_SDK_PATH="${WASI_SDK_PATH:-$PWD/tools/wasi-sdk}"
export PATH="$PWD/tools/bin:$PWD/tools/binaryen/bin:$PATH"
fail=0
out=$(mktemp)

need() { # name, hint, version-command...
  local name=$1 hint=$2; shift 2
  local v
  if ! v=$("$@" 2>&1 | head -1) || [ -z "$v" ]; then
    echo "MISSING  $name -- $hint" >&2; fail=1; return
  fi
  printf '%-22s %s\n' "$name" "$v" | tee -a "$out"
}

show() { # name, hint, version-command...: printed, not recorded (host-dependent)
  local name=$1 hint=$2; shift 2
  local v
  if ! v=$("$@" 2>&1 | head -1) || [ -z "$v" ]; then
    echo "MISSING  $name -- $hint" >&2; fail=1; return
  fi
  printf '%-22s %s\n' "$name" "$v"
}

need go "install Go (see go.mod)" go env GOVERSION
show node "install Node >= 22" node --version
show npm "ships with Node" npm --version
need clang "run 'make tools' or set WASI_SDK_PATH" "$WASI_SDK_PATH/bin/clang" --version
need wizer "run 'make tools'" bash -c 'wizer --help >/dev/null && echo "wizer (pinned '"$WIZER_VERSION"')"'
need wasm-opt "run 'make tools'" wasm-opt --version
need wasm2go "run 'make tools'" wasm2go -version
need faker "run 'make bundle'" bash -c 'echo "@faker-js/faker '"$FAKER_VERSION"' (pinned)"'
need quickjs-ng "pinned source, fetched by module build" bash -c 'echo "quickjs-ng '"$QUICKJS_NG_TAG"' '"$QUICKJS_NG_SHA"'"'
if command -v docker >/dev/null; then show docker "optional" docker --version; else echo "docker                 not installed (optional; cross-arch tests will be skipped)"; fi

[ $fail = 0 ] || { echo "doctor: toolchain incomplete" >&2; exit 1; }
cp "$out" tools.versions
echo "doctor: ok -> tools.versions"
