#!/usr/bin/env bash
# Downloads the pinned wasm toolchain into ./tools (git-ignored).
set -euo pipefail
cd -P -- "$(dirname -- "$0")/.."
. scripts/pins.env

case "$(uname -s)-$(uname -m)" in
  Linux-aarch64)  W=arm64-linux;  B=aarch64-linux; Z=aarch64-linux ;;
  Linux-x86_64)   W=x86_64-linux; B=x86_64-linux;  Z=x86_64-linux ;;
  Darwin-arm64)   W=arm64-macos;  B=arm64-macos;   Z=aarch64-macos ;;
  Darwin-x86_64)  W=x86_64-macos; B=x86_64-macos;  Z=x86_64-macos ;;
  *) echo "unsupported host $(uname -sm)" >&2; exit 1 ;;
esac

mkdir -p tools/dl tools/bin
if [ ! -x tools/wasi-sdk/bin/clang ]; then
  curl -fsSL "https://github.com/WebAssembly/wasi-sdk/releases/download/wasi-sdk-${WASI_SDK_VERSION}/wasi-sdk-${WASI_SDK_VERSION}.0-${W}.tar.gz" | tar xzC tools
  mv tools/wasi-sdk-* tools/wasi-sdk
fi
if [ ! -x tools/binaryen/bin/wasm-opt ]; then
  curl -fsSL "https://github.com/WebAssembly/binaryen/releases/download/version_${BINARYEN_VERSION}/binaryen-version_${BINARYEN_VERSION}-${B}.tar.gz" | tar xzC tools
  mv tools/binaryen-* tools/binaryen
fi
if [ ! -x tools/bin/wizer ]; then
  curl -fsSL "https://github.com/bytecodealliance/wizer/releases/download/v${WIZER_VERSION}/wizer-v${WIZER_VERSION}-${Z}.tar.xz" | tar xJC tools/dl
  cp tools/dl/wizer-*/wizer tools/bin/wizer
fi
if [ ! -x tools/bin/wasm2go ]; then
  root="$PWD"
  (cd /tmp && GOBIN="$root/tools/bin" go install "github.com/ncruces/wasm2go@${WASM2GO_VERSION}")
fi
