#!/bin/bash
# The fast quality suite for the monorepo: Go (vet, build, unit + integration),
# Angular (build, unit), the tooling self-tests and the API-contract drift guard.
#
# `make test` runs it. The mutation gates are NOT here - they are the Makefile's
# `backend-mutation` (gremlins, one package per cache dir, floor 85) and
# `frontend-mutation` (Stryker, thresholds.break = 85), so `make mutation` and a CI
# shard run the same command instead of two copies of the same loop.
#
# Local: a missing toolchain is SKIPPED (install Go, or run `npm ci` in frontend/).
# CI:    every toolchain is REQUIRED; a missing one FAILS (no silent skips).
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CI="${CI:-}"
GO_IMAGE="${GO_IMAGE:-golang:1.26-alpine}"

fail=0
ran=0
skipped=0

say() { printf '%s\n' "$*"; }
have() { command -v "$1" >/dev/null 2>&1; }

# Karma and Stryker need a Chrome binary. CI has one; locally take the Playwright
# chromium when CHROME_BIN is not already set (the Makefile does the same, this
# covers running the script directly).
if [[ -z ${CHROME_BIN:-} ]]; then
  for chrome in "$HOME"/.cache/ms-playwright/chromium-*/chrome-linux*/chrome; do
    [[ -x $chrome ]] && export CHROME_BIN="$chrome"
  done
fi

skip_or_fail() { # <tool> <reason>
  if [[ -n $CI ]]; then
    say "FAIL  missing tool in CI: $1 ($2)"; fail=1
  else
    say "SKIP  $1 not available locally ($2)"; skipped=$((skipped + 1))
  fi
}

# ── Go: vet + build + unit tests ────────────────────────────────────────────
if [[ -d $ROOT/backend ]]; then
  say "== Go: vet + build + unit + integration tests =="
  # The integration/e2e tests must not be skipped in CI: they need a database.
  if [[ -n $CI ]] && [[ -z ${DATABASE_URL:-} ]]; then
    say "FAIL  DATABASE_URL is required in CI (integration tests must not skip)"; fail=1
  fi
  if have go; then
    ( cd "$ROOT/backend" && go vet ./... && go build ./... && go test ./... )
    rc=$?
  elif have docker; then
    # A named module cache: without it the container re-downloads every module on
    # every run (the old fallback also passed GOPROXY=off, which made it a guaranteed
    # failure on a cold cache).
    docker run --rm -v "$ROOT/backend":/app -v campus-erp-gomod:/go/pkg/mod -w /app \
      -e GOPATH=/go -e GOCACHE=/tmp/gocache "$GO_IMAGE" \
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
    skip_or_fail npm "run 'make deps' (npm ci in frontend/)"
  fi
fi

# ── Tooling: ER-model generator self-test ───────────────────────────────────
if [[ -f $ROOT/tools/test_erm.py ]]; then
  say "== Tooling: ER-model generator self-test =="
  if have python3; then
    python3 "$ROOT/tools/test_erm.py"
    rc=$?
    if (( rc == 0 )); then ran=$((ran + 1)); else fail=1; say "FAIL  ER generator self-test"; fi
  else
    skip_or_fail python3 "install python3"
  fi
fi

# ── Tooling: OpenAPI generator self-test ────────────────────────────────────
if [[ -f $ROOT/tools/gen_openapi.py ]]; then
  say "== Tooling: OpenAPI generator self-test =="
  if have python3; then
    python3 "$ROOT/tools/gen_openapi.py" --selftest
    rc=$?
    if (( rc == 0 )); then ran=$((ran + 1)); else fail=1; say "FAIL  OpenAPI generator self-test"; fi
  else
    skip_or_fail python3 "install python3"
  fi
fi

# ── Tooling: API contract (drift guard + generated client) ──────────────────
if [[ -f $ROOT/tools/api_contract.py ]]; then
  say "== Tooling: API contract (drift guard) =="
  if have python3; then
    python3 "$ROOT/tools/api_contract.py" selftest && python3 "$ROOT/tools/api_contract.py" check
    rc=$?
    if (( rc == 0 )); then ran=$((ran + 1)); else fail=1; say "FAIL  API contract"; fi
  else
    skip_or_fail python3 "install python3"
  fi
fi

say ""
say "summary: $ran suite(s) ran, $skipped skipped, fail=$fail"
exit $fail
