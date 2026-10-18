#!/usr/bin/env bash
# Builds the IIFE bundle the module embeds (faker.en.min.js) from the npm package pinned in package-lock.json.
set -euo pipefail
cd -P -- "$(dirname -- "$0")"
mkdir -p ../reports
npm ci --silent --no-audit --no-fund
npx esbuild entry.en.js --bundle --minify --format=iife --target=es2020 \
  --platform=neutral --tree-shaking=true --drop:console --drop:debugger \
  --mangle-props="^_" --charset=utf8 --outfile=faker.en.min.js --log-level=warning
printf '%s raw=%s gzip=%s\n' faker.en.min.js "$(wc -c < faker.en.min.js)" "$(gzip -9c faker.en.min.js | wc -c)" \
  | tee ../reports/bundle-sizes.txt
