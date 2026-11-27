#!/bin/bash
# `make dev-setup`: turn a fresh checkout into a working local stack.
#
# It is idempotent. Every step looks before it acts, so running it again on a checkout that
# is already wired costs a few seconds and changes nothing — which is the point: a second
# developer (or a second day) should not have to remember which of these was skipped.
#
#   1. preflight  docker + compose v2 must answer; the optional toolchains are reported
#   2. deps       Go modules and npm packages (`make deps`), when they are not current
#   3. browser    the Playwright chromium `make test` and Stryker need
#   4. hooks      `.githooks` as core.hooksPath, so the policy check runs on commit
#   5. env        `.env` from `.env.example` when absent — never overwrites a real one
#   6. stack      postgres, api, worker, connectors, web and the local IdP, so the login
#                 screen is live (`make run-idp`); `--no-idp` leaves the stack open
#   7. summary    the addresses, the demo credentials and the commands that come next
#
# Not here: `make build` (the images build themselves in step 6) and the mutation gates
# (`make mutation` is a 45-minute sweep and belongs to the person landing a change).
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

with_idp=1
for arg in "$@"; do
  case $arg in
    --no-idp) with_idp=0 ;;
    -h | --help)
      # The header comment, without the shebang: the same text, one place.
      awk 'NR > 1 { if (!/^#/) exit; sub(/^# ?/, ""); print }' "$0"
      exit 0
      ;;
    *) printf 'error: unknown argument %s\n' "$arg" >&2; exit 2 ;;
  esac
done

say() { printf '%s\n' "$*"; }
step() { printf '\n==> %s\n' "$*"; }
have() { command -v "$1" >/dev/null 2>&1; }
die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

# ── 1. preflight ─────────────────────────────────────────────────────────────
step "preflight"
have docker || die "docker is required (docs/build-and-deploy.md)"
docker compose version >/dev/null 2>&1 ||
  die "docker compose v2 is required: the stack is driven with 'docker compose'"
docker info >/dev/null 2>&1 || die "the docker daemon is not answering"
say "  ok      docker $(docker version --format '{{.Server.Version}}')"

# Optional on purpose: every backend target falls back to the container, and `npm ci` is the
# only thing that really needs node. Reported, never fatal — a missing tool is not a broken
# checkout, and the step that needs it says so when it fails.
for tool in node npm python3 npx gremlins; do
  if have "$tool"; then say "  ok      $tool"; else say "  missing $tool (optional)"; fi
done
if go version >/dev/null 2>&1; then
  say "  ok      go $(go version | awk '{print $3}')"
else
  say "  notice  go is not runnable here; backend targets run it in the container"
fi

# ── 2. dependencies ─────────────────────────────────────────────────────────
step "dependencies"
# `npm ci` wipes node_modules, so only run it when the install is absent or older than the
# lock file. The same call refreshes the Go module cache, which is cheap when it is warm.
deps_stale=0
[[ -x $ROOT/frontend/node_modules/.bin/ng ]] || deps_stale=1
[[ -f $ROOT/frontend/node_modules && $ROOT/frontend/package-lock.json -nt $ROOT/frontend/node_modules ]] && deps_stale=1
if ((deps_stale)); then
  make -C "$ROOT" --no-print-directory deps || die "make deps failed"
else
  say "  ok      npm packages and Go modules are current"
fi

# ── 3. test browser ─────────────────────────────────────────────────────────
step "test browser"
# Karma, Stryker and the e2e suite all drive the Playwright chromium; the Makefile picks it up
# as CHROME_BIN when it is on disk, so this is the difference between `make test` working and
# every frontend target complaining about a missing browser.
playwright_chromium="$(ls -d "$HOME"/.cache/ms-playwright/chromium-* 2>/dev/null | tail -1)"
if [[ -n $playwright_chromium ]]; then
  say "  ok      $(basename "$playwright_chromium")"
else
  make -C "$ROOT" --no-print-directory frontend-e2e-install || die "installing the test browser failed"
fi

# ── 4. git hooks ────────────────────────────────────────────────────────────
step "git hooks"
if [[ $(git -C "$ROOT" config --get core.hooksPath || true) == ".githooks" ]]; then
  say "  ok      core.hooksPath=.githooks"
else
  make -C "$ROOT" --no-print-directory hooks || die "enabling the hooks failed"
fi

# ── 5. environment file ─────────────────────────────────────────────────────
step "environment file"
if [[ -f $ROOT/.env ]]; then
  say "  ok      .env exists (left alone)"
else
  cp "$ROOT/.env.example" "$ROOT/.env"
  say "  wrote   .env from .env.example"
fi
# The summary below quotes the ports the stack will actually use, so read the same file
# compose reads. It is the developer's own file: same trust as the Makefile's `-include`.
# shellcheck disable=SC1091
set -a; . "$ROOT/.env"; set +a

# ── 6. the stack ────────────────────────────────────────────────────────────
step "local stack"
if ((with_idp)); then
  make -C "$ROOT" --no-print-directory run-idp || die "starting the stack failed"
else
  make -C "$ROOT" --no-print-directory run || die "starting the stack failed"
fi

# ── 7. what now ─────────────────────────────────────────────────────────────
step "ready"
say "  web       http://localhost:${WEB_PORT:-8081}"
say "  api       http://localhost:${API_PORT:-8080}  (readiness: /readyz)"
say "  postgres  localhost:${POSTGRES_PORT:-5433}  -  'make db-shell' for a psql prompt"
if ((with_idp)); then
  say "  idp       http://localhost:${KEYCLOAK_PORT:-9080}/realms/${IDP_REALM:-campus-erp}"
  say "            sign in as max.maag / kasse"
fi
say "  shell     'make shell' for a shell inside the api container"
say ""
say "next:  make test      # the fast quality suite"
say "       make logs      # tail the stack (make logs S=api)"
say "       make stop      # stop it, keeping the database volume"
