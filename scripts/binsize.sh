#!/usr/bin/env bash
# E6: stripped binary size and cold `go build` time (build cache cleared) for an empty main, the native engine
# (en snapshot) and the wazero baseline. v8go is not built here.
set -uo pipefail
cd -P -- "$(dirname -- "$0")/../bench" || exit
mkdir -p ../build/binsize
printf '%-16s %12s %10s %12s\n' target bytes build_s rss_MB
t() { # name tags pkg
  go clean -cache
  local s e
  s=$(date +%s.%N)
  /usr/bin/time -f "%M" -o ../build/binsize/rss go build -tags "$2" -ldflags='-s -w' -o "../build/binsize/$1" "$3" || return
  e=$(date +%s.%N)
  printf '%-16s %12s %10.1f %12s\n' "$1" "$(stat -c %s "../build/binsize/$1")" "$(echo "$e - $s" | bc)" "$(( $(cat ../build/binsize/rss) / 1024 ))"
}
t empty "" ./cmd/empty
t native-en "" ./cmd/native
t baseline-qjs "" ./cmd/qjs
