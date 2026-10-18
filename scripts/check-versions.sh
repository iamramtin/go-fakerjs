#!/usr/bin/env bash
# Fails if any place that states the current Faker.js version disagrees with tools.versions, its single source
# of truth, or if any tracked or untracked non-ignored file still holds a placeholder token.
# Documents state the current version on the lines listed by doc_pins; other versions in FINDINGS.md are
# historical (the version a result was measured on) and are left alone.
#
# Usage: scripts/check-versions.sh             run every check
#        scripts/check-versions.sh --doc-pins  print the doc_pins table (scripts/faker-upgrade.sh rewrites it)
set -euo pipefail
cd -P -- "$(dirname -- "$0")/.."

# One line per statement of the current version: file, a tab, and the ERE that immediately precedes the version.
# faker-upgrade.sh substitutes with sed and # as the delimiter, so an ERE must not contain #.
doc_pins() {
  printf '%s\t%s\n' \
    README.md '^\| v[0-9][0-9.]* \|[ ]' \
    CONTRIBUTING.md '^\| Faker\.js \|[ ]' \
    THIRD_PARTY_NOTICES.md '^- \*\*Faker\.js[ ]' \
    THIRD_PARTY_NOTICES.md '^\| Faker\.js \(.@faker-js/faker.\) \| [^|]* \|[ ]' \
    FINDINGS.md 'The current pin is Faker\.js[ ]'
}
if [ "${1:-}" = --doc-pins ]; then doc_pins; exit 0; fi

. scripts/pins.env
v=$FAKER_VERSION
fail=0 checks=0
check() { # description, actual
  checks=$((checks + 1))
  if [ "$2" != "$v" ]; then echo "check-versions: $1 is '$2', tools.versions says '$v'" >&2; fail=1; fi
}
json() { node -p "const o = require('./$1'); $2"; }

for d in bundle golden; do
  check "$d/package.json dependency" "$(json "$d/package.json" 'o.dependencies["@faker-js/faker"]')"
  check "$d/package-lock.json resolved version" "$(json "$d/package-lock.json" 'o.packages["node_modules/@faker-js/faker"].version')"
done
check "golden/api.json fakerVersion" "$(json golden/api.json o.fakerVersion)"
check "golden/testdata/meta.json fakerVersion" "$(json golden/testdata/meta.json o.fakerVersion)"
check "fakerjs.FakerVersion" "$(sed -n 's/^const FakerVersion = "\(.*\)"$/\1/p' fakerjs.go)"

while IFS=$'\t' read -r f re; do
  found=$(grep -oE "${re}[0-9][0-9.]*[0-9]" "$f" | grep -oE '[0-9][0-9.]*[0-9]$' || true)
  if [ -z "$found" ]; then
    checks=$((checks + 1))
    echo "check-versions: $f has no line matching /$re/ followed by a version" >&2; fail=1; continue
  fi
  while IFS= read -r m; do check "$f current version" "$m"; done <<<"$found"
done < <(doc_pins)

check "NOTICE" "$(sed -n 's/.*includes Faker\.js \([0-9][0-9.]*\) .*/\1/p' NOTICE)"
first=$(awk '/^## /{n++} n==1' CHANGELOG.md)
checks=$((checks + 1))
grep -q "Faker\.js $v" <<<"$first" || { echo "check-versions: the first CHANGELOG section does not mention Faker.js $v" >&2; fail=1; }

# Placeholder tokens. Each is spelled with a bracket expression so this file does not match itself; the word
# tokens must stand alone, so a word that only contains one of them does not match.
w='(^|[^A-Za-z0-9_])'
e='($|[^A-Za-z0-9_])'
placeholders="<<[A-Z][A-Za-z0-9_ .-]*>>|${w}T[B]D${e}|${w}T[O]DO${e}|${w}X[X]X${e}|${w}v0\.[x]${e}|x\.y\.[z]|<M[O]DEL|<D[A]TE"
checks=$((checks + 1))
if git rev-parse --git-dir >/dev/null 2>&1; then
  # -s: a path staged but deleted from the working tree is not an error. xargs folds grep's exit status, so the
  # output decides.
  hits=$(git ls-files -z --cached --others --exclude-standard | xargs -0 grep -snIE "$placeholders" -- || true)
  if [ -n "$hits" ]; then
    echo "check-versions: placeholder tokens found:" >&2
    echo "$hits" >&2
    fail=1
  fi
else
  echo "check-versions: not a git checkout, placeholder scan skipped" >&2
fi

[ $fail = 0 ] || exit 1
echo "check-versions: ok ($checks checks, Faker.js $v)"
