#!/bin/bash
# Full quality suite for the monorepo (ADR-0008).
#
# Mutation floor is 0.85 and is enforced by the mutation tools themselves:
#   - Go:  gremlins --threshold-efficacy 85   (per package)
#   - TS:  stryker thresholds.break = 85      (frontend/stryker.conf.json)
#
# NOTE: gremlins does NOT accept Go "..." patterns - `gremlins unleash ./internal/...`
# silently reports "No results to report." and exits 0 (a vacuous pass). So the Go
# mutation step ENUMERATES the packages from backend/mutation.json and FAILS on a
# run that produced no mutants.
#
# Local: falls back to docker for Go when the toolchain is absent.
# CI:    every toolchain is REQUIRED; a missing one FAILS (no silent skips).
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CI="${CI:-}"
GO_IMAGE="${GO_IMAGE:-golang:1.25-alpine}"
MUTATION_THRESHOLD="${MUTATION_THRESHOLD:-85}"

fail=0
ran=0
skipped=0

say() { printf '%s\n' "$*"; }
have() { command -v "$1" >/dev/null 2>&1; }

skip_or_fail() { # <tool> <reason>
  if [[ -n $CI ]]; then
    say "FAIL  missing tool in CI: $1 ($2)"; fail=1
  else
    say "SKIP  $1 not available locally ($2)"; skipped=$((skipped + 1))
  fi
}

mutation_targets() { # package paths from backend/mutation.json
  python3 - "$ROOT/backend/mutation.json" <<'PY'
import json, sys
for t in json.load(open(sys.argv[1])).get("targets", []):
    print(t["path"])
PY
}

# ── Go: vet + build + unit tests ────────────────────────────────────────────
if [[ -d $ROOT/backend ]]; then
  say "== Go: vet + build + unit tests =="
  if have go; then
    ( cd "$ROOT/backend" && go vet ./... && go build ./... && go test ./... )
    rc=$?
  elif have docker; then
    docker run --rm -v "$ROOT/backend":/app -w /app -e GOPROXY=off "$GO_IMAGE" \
      sh -c 'go vet ./... && go build ./... && go test ./...'
    rc=$?
  else
    skip_or_fail go "install Go or docker"; rc=1
  fi
  if (( rc == 0 )); then ran=$((ran + 1)); else fail=1; say "FAIL  Go suite"; fi
fi

# ── Angular: build + unit tests ─────────────────────────────────────────────
if [[ -d $ROOT/frontend ]]; then
  say "== Angular: build + unit tests =="
  if [[ -d $ROOT/frontend/node_modules ]] && have npx; then
    ( cd "$ROOT/frontend" && npx ng build --configuration development >/dev/null && \
      npx ng test --watch=false )   # browsers come from karma.conf.js
    rc=$?
    if (( rc == 0 )); then ran=$((ran + 1)); else fail=1; say "FAIL  Angular suite"; fi
  else
    skip_or_fail npm "run 'npm ci' in frontend/"
  fi
fi

# ── Mutation: Go (gremlins, per package) ────────────────────────────────────
say "== Mutation: Go (floor ${MUTATION_THRESHOLD}%) =="
if [[ -d $ROOT/backend ]]; then
  if have gremlins; then
    targets="$(mutation_targets)"
    [[ -z $targets ]] && { say "FAIL  mutation/go no targets in mutation.json"; fail=1; }
    while read -r pkg; do
      [[ -z $pkg ]] && continue
      out="$( cd "$ROOT/backend" && gremlins unleash --threshold-efficacy "$MUTATION_THRESHOLD" "./$pkg" 2>&1 )"
      rc=$?
      if grep -q "No results to report" <<<"$out"; then
        say "FAIL  mutation/go $pkg produced no mutants (vacuous run)"; fail=1
      elif (( rc != 0 )); then
        say "FAIL  mutation/go $pkg below floor"; fail=1
      else
        say "PASS  mutation/go $pkg"
      fi
    done <<<"$targets"
    ran=$((ran + 1))
  else
    skip_or_fail gremlins "go install github.com/go-gremlins/gremlins/cmd/gremlins@latest"
  fi
fi

# ── Mutation: TypeScript (stryker) ──────────────────────────────────────────
say "== Mutation: TypeScript (floor ${MUTATION_THRESHOLD}%) =="
if [[ -f $ROOT/frontend/stryker.conf.json ]]; then
  if [[ -d $ROOT/frontend/node_modules ]] && have npx; then
    ( cd "$ROOT/frontend" && npx stryker run )
    rc=$?
    if (( rc == 0 )); then ran=$((ran + 1)); else fail=1; say "FAIL  mutation/web below floor"; fi
  else
    skip_or_fail npx "npm ci in frontend/"
  fi
fi

say ""
say "summary: $ran suite(s) ran, $skipped skipped, fail=$fail"
exit $fail
