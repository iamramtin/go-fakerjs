#!/usr/bin/env bash
# Maintainer release driver for the macOS host. Run from anywhere inside the repository.
#
#   scripts/host/release.sh           (same as: release.sh bench)
#       Benchmarks the tree, merges the results into main through a pull request, and waits for the required
#       checks. Runs unattended for about two and a half hours. Keeps the Mac awake throughout.
#
#   scripts/host/release.sh tag
#       Run after the regenerated tables are merged into README.md and FINDINGS.md and CHANGELOG.md has a dated
#       v0.1.0 heading. Refuses otherwise. Waits for CI on main, creates the annotated tag, pushes it, and checks
#       that the module resolves.
#
# Between the two stages you edit the documents yourself: tables.py prints Markdown and does not write files.
# main accepts changes only through pull requests (repository ruleset), so every write below goes through one.
# A pushed tag cannot be moved or deleted, and the Go module proxy caches it permanently.
set -euo pipefail

readonly VERSION=v0.1.0
readonly REPO=iamramtin/go-fakerjs

# Keep the Mac awake for the whole run. Must come before the cd below, so a relative $0 still resolves.
if [ -z "${RELEASE_CAFFEINATED:-}" ] && command -v caffeinate >/dev/null 2>&1; then
  RELEASE_CAFFEINATED=1 exec caffeinate -dimsu "$0" "$@"
fi
cd -P -- "$(dirname -- "$0")/../.."

log() { printf '%s release.sh: %s\n' "$(date +%H:%M:%S)" "$*"; }
die() { log "STOP: $*" >&2; exit 1; }

preflight() {
  [ "$(uname -s)/$(uname -m)" = Darwin/arm64 ] || die "run on the macOS host (darwin/arm64)"
  [ "$(git symbolic-ref --short HEAD)" = main ] || die "not on main; run: git switch main && git pull --ff-only"
  git fetch -q origin
  [ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || die "main differs from origin/main; run: git pull --ff-only"
  git diff --quiet && git diff --cached --quiet || die "tracked files have uncommitted changes"
  [ -z "$(git ls-files --others --exclude-standard)" ] || die "untracked files present; commit or remove them"
}

# wait_pr BRANCH: waits until the pull request for BRANCH is merged. Stops early if a required check fails.
wait_pr() {
  local branch=$1 state failed i url
  log "waiting for checks and merge of $branch"
  for i in $(seq 1 300); do # five hours at 60 s
    state=$(gh pr view "$branch" --repo "$REPO" --json state --jq .state)
    case $state in
      MERGED) log "merged"; return 0 ;;
      CLOSED) die "pull request for $branch was closed without merging" ;;
    esac
    failed=$(gh pr view "$branch" --repo "$REPO" --json statusCheckRollup \
      --jq '[(.statusCheckRollup // [])[] | select((.conclusion // .state) | IN("FAILURE","ERROR","TIMED_OUT","CANCELLED","ACTION_REQUIRED","STARTUP_FAILURE"))] | length')
    if [ "$failed" -gt 0 ]; then
      url=$(gh pr view "$branch" --repo "$REPO" --json url --jq .url)
      die "a required check failed on $branch: $url"
    fi
    sleep 60
  done
  die "timed out waiting for $branch to merge"
}

# wait_ci SHA: waits for the ci run on SHA to complete, and fails unless it succeeded.
wait_ci() {
  local sha=$1 line status concl id i
  log "waiting for CI on $sha"
  for i in $(seq 1 300); do
    line=$(gh run list --repo "$REPO" --branch main --limit 20 \
      --json headSha,status,conclusion,databaseId,workflowName \
      --jq ".[] | select(.workflowName == \"ci\" and .headSha == \"$sha\") | \"\(.status) \(.conclusion) \(.databaseId)\"" |
      head -1)
    if [ -n "$line" ]; then
      read -r status concl id <<<"$line"
      if [ "$status" = completed ]; then
        [ "$concl" = success ] || die "CI $concl on $sha: https://github.com/$REPO/actions/runs/$id"
        log "CI success on $sha (run $id)"
        return 0
      fi
    fi
    sleep 60
  done
  die "timed out waiting for CI on $sha"
}

stage_bench() {
  preflight
  local date_tag branch dir
  date_tag=$(date +%F)
  branch=release/benchmarks-$date_tag
  dir=results/$date_tag-apple-m5
  [ ! -e "$dir" ] || die "$dir already exists; move it aside first"

  log "benchmarks on $(git rev-parse --short HEAD): about an hour"
  scripts/host/bench-all.sh # also writes the darwin/arm64 golden dump hash into $dir
  python3 scripts/tables.py bench "$dir" >"$dir/tables.md"

  git switch -q -c "$branch"
  git add "$dir"
  git commit -q -m "perf: Record benchmarks on the final build"
  git push -q -u origin "$branch"
  gh pr create --repo "$REPO" --base main --head "$branch" \
    --title "perf: Record benchmarks on the final build" \
    --body "Benchmark results for $VERSION on the final tree. Tables: \`$dir/tables.md\`."
  gh pr merge --repo "$REPO" --squash --auto "$branch"
  wait_pr "$branch"

  git switch -q main
  git pull -q --ff-only

  log "results are on main. Next, by hand: put $dir/tables.md into README.md and FINDINGS.md, and replace"
  log "the CHANGELOG.md heading with '## $VERSION (YYYY-MM-DD)'. Merge that through a pull request, then run:"
  log "  scripts/host/release.sh tag"
}

stage_tag() {
  preflight

  local dir
  dir=$(ls -d results/*-apple-m5 2>/dev/null | sort | tail -1)
  [ -n "$dir" ] && [ -f "$dir/tables.md" ] || die "no results directory with tables.md"

  grep -qE "^## $VERSION \([0-9]{4}-[0-9]{2}-[0-9]{2}\)$" CHANGELOG.md ||
    die "CHANGELOG.md: the $VERSION heading has no release date yet"

  # Every table row generated from the results must already appear verbatim in README.md or FINDINGS.md on main.
  local row missing=0
  while IFS= read -r row; do
    case $row in '|---'*) continue ;; esac
    if ! grep -qF -- "$row" README.md FINDINGS.md; then
      log "not in README.md or FINDINGS.md: $row"
      missing=$((missing + 1))
    fi
  done < <(grep '^|' "$dir/tables.md")
  [ "$missing" -eq 0 ] || die "$missing table row(s) missing; merge the tables from $dir/tables.md first"

  wait_ci "$(git rev-parse HEAD)"

  git tag -a "$VERSION" -m "$VERSION"
  git push -q origin "$VERSION"
  log "pushed $VERSION at $(git rev-parse --short HEAD)"

  # The tag is public from here. If this fails, do not move the tag; release a fix as v0.1.1.
  (
    cd "$(mktemp -d)"
    go mod init example.com/release-check >/dev/null 2>&1
    go get "github.com/$REPO@$VERSION"
  ) || die "github.com/$REPO@$VERSION does not resolve; do not retag, release a fix as v0.1.1"

  log "module resolves at $VERSION. Once pkg.go.dev lists it, set the homepage:"
  log "  gh repo edit $REPO --homepage https://pkg.go.dev/github.com/$REPO"
}

case ${1:-bench} in
  bench) stage_bench ;;
  tag) stage_tag ;;
  *)
    echo "usage: $0 [bench|tag]" >&2
    exit 2
    ;;
esac
