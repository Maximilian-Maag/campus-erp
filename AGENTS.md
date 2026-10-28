# AGENTS.md — repository rules

Rules for anyone (human or agent) working in this repository. These are the source
of the executable checks in `tools/policy_check.py` + `policy.json`, and of the
quality gate in `tools/run_tests.sh` + `.github/workflows/quality.yml`.

## What this repo is

An open-source, multi-tenant drop-in replacement for the HIS eG Hochschul-ERP
(Go backend, Angular frontend), plus the reverse-engineered analysis of the legacy
system it replaces.

## Hard rules

1. **Never commit the legacy dump.** `*.backup` is git-ignored; it is proprietary.
2. **Never commit personal or financial data.** Use synthetic fixtures.
3. **Mark inference.** Claims about the legacy system cite evidence or are marked
   *(inferred)*. See `docs/01..04`.
4. **Regenerate, don't hand-edit.** `schema/` is produced by `scripts/`.
5. **Contract-first.** The OpenAPI spec (`contracts/`) is the interface between
   backend and frontend; the Angular client is generated, never hand-written.
6. **Compliance is a constraint, not a feature.** Every state change goes through
   the audit package (ADR-0006). No hard deletes on business data.
7. **Tenant isolation.** No query path may cross tenants (ADR-0002).
8. **No absolute home paths** in tracked files.
9. **Shell style:** `#!/bin/bash`, `[[ ]]` (unquoted vars), `(( ))` for numbers.

## Quality gate (ADR-0008)

- Unit, integration and e2e tests are **mandatory**.
- **Mutation floor is 0.85**, enforced by the tools themselves:
  - Go: `gremlins unleash --threshold-efficacy 85 ./<package>` **per package**
    (`./internal/...` is a vacuous pass — gremlins reports "No results" and exits 0;
    `tools/run_tests.sh` enumerates `backend/mutation.json` and fails on an empty run)
  - TS: `stryker run` with `thresholds.break = 85` (`frontend/stryker.conf.json`)
- Run everything: `tools/run_tests.sh`. In CI every toolchain is required; a
  missing tool **fails** (no silent skips).
- A scored target is run three times and must report identical numbers before a
  score is quoted.
- **Run gremlins one package per cache dir.** gremlins caches its coverage profile
  in the user cache dir; a shared cache across packages produces wrong,
  non-reproducible scores (a package once reported 0 killed after another had run).
  `tools/run_tests.sh` and CI set `XDG_CACHE_HOME` per package.
- **Never derive test-uniqueness from a timestamp.** The test database persists
  between runs and is shared by packages Go runs in parallel, so a modulo-reduced
  `time.Now()` repeats and collides on unique constraints. Use the database
  (`testSeqNext`, a sequence) — it is the only source unique across packages *and*
  runs. `CREATE SEQUENCE IF NOT EXISTS` is not atomic either.
- **The API contract is enforced.** `tools/api_contract.py check` fails if a Go
  route is undocumented or a documented route is missing from Go; CI also
  regenerates the client and the spec and fails on any diff. Add a route in
  `httpapi/server.go` **and** a row in the `CONTRACT` table of
  `tools/gen_openapi.py`, then `make contracts-gen` (`docs/15`).
- `db.MigrationsFrom`'s sort comparator (`Version < Version`) is an **equivalent**
  mutant: duplicate versions are rejected before the sort, so `<` and `<=` order
  identically. The surviving mutant is expected and documented, not a gap.

## Build, package & deploy

`make` is the single entry point (see `docs/build-and-deploy.md`):

- `make all` — C4 PDF + backend + frontend + container images.
- `make test` — the quality suite (`tools/run_tests.sh`).
- `make c4-pdf` — `c4/workspace.dsl` → `c4/export/campus-erp-c4-model.pdf`
  (`tools/c4_pdf.sh`). The Structurizr image cannot render PNG locally, so the
  script exports PlantUML and renders with `plantuml/plantuml`; both containers
  run as `--user $(id -u):$(id -g)`.
- `make docker-images` / `make up` / `make deploy-backend|deploy-frontend` —
  images and the docker-compose stack. `deploy-backend` + `workon-frontend`
  (and the reverse) let one half be deployed while the other is edited.
- `make helm-lint` / `make helm-template` / `make deploy-k8s` — the chart in
  `deploy/helm/campus-erp` (ADR-0007).

The backend image builds all three roles (api/worker/connectors) from one
tree; role is selected by the container command. The frontend image is
Angular built once, served by nginx with `/api` proxied same-origin.

## Policy as code

- `tools/policy_check.py` (vendored; checker version pinned in `policy.json`).
- `.githooks/pre-commit` → `--changed`. Enable: `git config core.hooksPath .githooks`.
- `.github/workflows/policy.yml` → `--ci`.

### Why `policy.json` has no `tests` section

The vendored checker's `tests` rules are built for the Omarchy **plugin** layout
(they require a Python `tools/mutator.py` and `tests/test_<kind>_*.py`). This is a
Go + Angular monorepo, so those rules do not apply; the quality floor is enforced
instead by `tools/run_tests.sh`, `frontend/stryker.conf.json`, `backend/mutation.json`
and `.github/workflows/quality.yml`. `policy_check` therefore reports
`tests/layout` as a WARN by design.

## Re-vendoring the checker

```bash
cp ~/Work/omarchy-qutebrowser/tools/policy_check.py tools/policy_check.py
# then set "checker" in policy.json to the file's CHECKER_VERSION
```
