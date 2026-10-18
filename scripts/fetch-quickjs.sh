#!/usr/bin/env bash
# Fetches the pinned QuickJS-NG source tree into build/quickjs-ng (git-ignored).
set -euo pipefail
cd -P -- "$(dirname -- "$0")/.."
. scripts/pins.env

dest=build/quickjs-ng
if [ -f "$dest/.sha" ] && [ "$(cat "$dest/.sha")" = "$QUICKJS_NG_SHA" ]; then
  exit 0
fi
rm -rf "$dest"
mkdir -p "$dest"
curl -fsSL "https://github.com/quickjs-ng/quickjs/archive/${QUICKJS_NG_SHA}.tar.gz" | tar xz -C "$dest" --strip-components=1
echo "$QUICKJS_NG_SHA" > "$dest/.sha"
