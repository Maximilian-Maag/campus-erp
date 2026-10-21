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
