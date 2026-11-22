# Contributing

## Before you start

- `AGENTS.md` is the working agreement for this repository (it applies to humans and
  to agents alike); `docs/build-and-deploy.md` covers the toolchain.
- Architectural decisions live in `docs/adr/`; the delivery plan in `docs/plan/`.

## The gate

Every change must pass:

```bash
make test        # Go vet/build/unit+integration, Angular build + unit, tooling, contract
make mutation    # gremlins per package + Stryker, floor 0.85
```

plus `make policy` (style, hygiene and secret rules) and `make contracts-check` (the
OpenAPI spec must match the Go routes). `make quality` runs all of it; CI runs the same
targets through the same Makefile.

## Conventions

- **Contract-first**: add the route, add the contract row in `tools/gen_openapi.py`,
  then `make contracts-gen`.
- Migrations are append-only, under `backend/internal/platform/db/migrations/`.
- Never commit secrets; local defaults belong in `.env.example` / `docker-compose.yml`.
- **No copy in components.** Every word a screen shows is a message key in
  `frontend/src/app/i18n/` — a shared key if the term already exists, otherwise the area's
  fragment — in German *and* English. `npm run i18n` (part of `make frontend-test`) fails on a
  hard-coded string and on a key that names no entry; see
  [`docs/32-design-system-and-i18n.md`](docs/32-design-system-and-i18n.md).
- Do not commit proprietary third-party artefacts - see "What is not in this
  repository" in the README.

## Issues and pull requests

Open an issue first for anything non-trivial; then a topic branch, a PR that closes
the issue, and a green run before merging.

**Target `dev`, not `master`.** `dev` is the integration branch and collects every feature;
`master` is the release line and moves only by a promotion PR (`dev` → `master`) that carries a
batch of features to a release. The image channels follow that: a push to `dev` publishes
`latest`, `staging` publishes the release candidate, and prod is the `vX.Y.Z` tag cut on
`master` (see "Images" in [`docs/build-and-deploy.md`](docs/build-and-deploy.md)).
