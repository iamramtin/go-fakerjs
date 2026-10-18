#!/usr/bin/env bash
# Upgrades the pinned Faker.js to a newer minor or patch release and proves the result before committing it.
#
# Usage:
#   scripts/faker-upgrade.sh                                adopt the latest release if it is a newer minor or patch
#   TARGET=<version> scripts/faker-upgrade.sh --dry-run     run the whole pipeline for <version>; commit nothing
#
# The work happens in a temporary git worktree (detached at HEAD), so the current checkout is never touched.
# Steps: pin, bundle, golden data (from the new Faker), snapshot (clang, wasm-opt, Wizer, wasm2go), check-versions,
# the test suite (conformance, contract, stress -short, fuzz smoke) and the reproducibility check.
# On success (not a dry run) the change is committed on branch faker-upgrade/<version>; nothing is pushed.
# On failure the worktree is removed, no branch is created, and the failing step and its log are named.
#
# Exit status: 0 up to date, upgraded, or dry run passed; 1 a step failed; 3 the release is a new major version,
# which is never adopted automatically (the upgrade workflow opens an issue instead).
set -euo pipefail
cd -P -- "$(dirname -- "$0")/.."
root=$PWD

dry=0
case "${1:-}" in
  --dry-run) dry=1 ;;
  "") ;;
  *) echo "usage: [TARGET=<version>] $0 [--dry-run]" >&2; exit 2 ;;
esac

. scripts/pins.env
cur=$FAKER_VERSION
if [ -n "${TARGET:-}" ]; then
  new=$TARGET
else
  new=$(npm view @faker-js/faker version)
fi
[[ $new =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "faker-upgrade: '$new' is not a release version" >&2; exit 1; }

if [ -z "${TARGET:-}" ]; then
  if [ "$new" = "$cur" ] || [ "$(printf '%s\n%s\n' "$cur" "$new" | sort -V | tail -1)" = "$cur" ]; then
    echo "faker-upgrade: up to date (Faker.js $cur; latest published is $new)"
    exit 0
  fi
fi
if [ "${new%%.*}" != "${cur%%.*}" ]; then
  echo "faker-upgrade: Faker.js $new is a new major version (current $cur). Major upgrades are not adopted" >&2
  echo "automatically: read the Faker.js migration guide, then upgrade by hand (CONTRIBUTING.md)." >&2
  exit 3
fi

logs="${TMPDIR:-/tmp}/faker-upgrade-$new-$$"
mkdir "$logs"
wt="$logs/worktree"
branch="faker-upgrade/$new"
git rev-parse --verify -q HEAD >/dev/null || { echo "faker-upgrade: the repository has no commits" >&2; exit 1; }
if [ $dry = 0 ] && git rev-parse --verify -q "refs/heads/$branch" >/dev/null; then
  echo "faker-upgrade: branch $branch already exists" >&2; exit 1
fi

done_ok=0
cleanup() {
  git -C "$root" worktree remove --force "$wt" >/dev/null 2>&1 || true
  git -C "$root" worktree prune >/dev/null 2>&1 || true
  [ $done_ok = 1 ] && rm -rf "$logs"
  return 0
}
trap cleanup EXIT

step() { # name, command...: runs in the worktree, output to $logs/<name>.log
  local name=$1; shift
  echo "==> $name"
  if ! (cd "$wt" && "$@") > "$logs/$name.log" 2>&1; then
    echo "faker-upgrade: FAILED at step '$name' (Faker.js $cur -> $new). Log: $logs/$name.log" >&2
    tail -20 "$logs/$name.log" >&2
    exit 1
  fi
}

echo "faker-upgrade: Faker.js $cur -> $new$([ $dry = 1 ] && echo ' (dry run)'), worktree $wt"
git worktree add -q --detach "$wt" HEAD
ln -s "$root/tools" "$wt/tools"
cp "$wt/golden/api.json" "$logs/api.old.json"

pin() {
  sed -i.bak "s/^\(faker  *@faker-js\/faker \)[0-9][0-9.]*\( .*\)$/\1$new\2/" tools.versions && rm tools.versions.bak
  (cd bundle && npm install --silent --no-audit --no-fund --save-exact "@faker-js/faker@$new")
  (cd golden && npm install --silent --no-audit --no-fund --save-exact "@faker-js/faker@$new")
  sed -i.bak "s/^const FakerVersion = \".*\"$/const FakerVersion = \"$new\"/" fakerjs.go && rm fakerjs.go.bak
  while IFS=$'\t' read -r f re; do
    sed -E -i.bak "s#(${re})[0-9][0-9.]*[0-9]#\1$new#" "$f" && rm "$f.bak"
  done < <(./scripts/check-versions.sh --doc-pins)
  sed -i.bak "s/includes Faker\.js $cur /includes Faker.js $new /" NOTICE && rm NOTICE.bak
}
step pin pin
step bundle make bundle
step golden-update make golden-update
step snapshot make snapshot

diffs() {
  node - "$logs/api.old.json" golden/api.json <<'JS'
    const path = require("path"); const [a, b] = [process.argv[2], process.argv[3]].map((f) => require(path.resolve(f)));
    const flat = (x) => Object.fromEntries(Object.entries(x.modules).flatMap(([m, ms]) => ms.map((f) => [m + "." + f, (x.arity || {})[m + "." + f]])));
    const [o, n] = [flat(a), flat(b)];
    const added = Object.keys(n).filter((k) => !(k in o)), removed = Object.keys(o).filter((k) => !(k in n));
    const changed = Object.keys(n).filter((k) => k in o && JSON.stringify(o[k]) !== JSON.stringify(n[k]));
    const list = (xs) => (xs.length ? xs.join(", ") : "none");
    console.log(`API diff (golden/api.json): ${added.length} added, ${removed.length} removed, ${changed.length} changed arity.`);
    console.log(`- Added: ${list(added)}`); console.log(`- Removed: ${list(removed)}`); console.log(`- Changed: ${list(changed)}`);
JS
}
apidiff() { diffs > "$logs/api-diff.txt"; }
step api-diff apidiff
api_summary=$(head -1 "$logs/api-diff.txt")

changelog() {
  local entry="- Upgrade to Faker.js $new (from $cur). $api_summary"
  if grep -q '^## Unreleased' CHANGELOG.md; then
    awk -v e="$entry" '{print} /^## Unreleased/ && !done {getline; print; print e; done=1}' CHANGELOG.md > CHANGELOG.new
  else
    awk -v e="$entry" '/^## / && !done {print "## Unreleased\n\n" e "\n"; done=1} {print}' CHANGELOG.md > CHANGELOG.new
  fi
  mv CHANGELOG.new CHANGELOG.md
}
step changelog changelog
step check-versions ./scripts/check-versions.sh
step vet go vet ./...
step test go test -count=1 -timeout 30m ./conformance ./contract .
step stress go test -count=1 -short -timeout 30m ./stress
step fuzz make test-fuzz FUZZTIME=60s
step snapshot-repro ./scripts/snapshot-repro.sh en

conf() {
  node - "$wt/reports/conformance-native-en.json" <<'JS'
    const c = require(process.argv[2]); const t = Object.entries(c.byCategory).sort();
    console.log("Conformance (en snapshot):"); for (const [k, v] of t) console.log(`- ${k}: ${v.Exact} exact, ${v.Normalized} normalized, ${v.Divergent} divergent`);
JS
}
conf > "$logs/conformance.txt"

msg="$logs/commit-message.txt"
{
  echo "chore: Upgrade Faker.js to $new"
  echo
  cat "$logs/api-diff.txt"
  echo
  cat "$logs/conformance.txt"
} > "$msg"

if [ $dry = 1 ]; then
  echo "faker-upgrade: dry run passed for Faker.js $new. Nothing committed. The commit would be:"
  echo "----"; cat "$msg"; echo "----"
  git -C "$wt" status --short | grep -v ' tools$' || true
  done_ok=1
  exit 0
fi

git -C "$wt" switch -q -c "$branch"
git -C "$wt" add -u
git -C "$wt" commit -q -F "$msg"
echo "faker-upgrade: committed $(git -C "$wt" rev-parse --short HEAD) on branch $branch"
done_ok=1
