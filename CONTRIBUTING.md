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

plus `python3 tools/policy_check.py --changed` (style, hygiene and secret rules) and
`python3 tools/api_contract.py check` (the OpenAPI spec must match the Go routes).
`make test-all` runs both halves locally; CI runs the same checks.

## Conventions

- **Contract-first**: add the route, add the contract row in `tools/gen_openapi.py`,
  then `make contracts-gen`.
- Migrations are append-only, under `backend/internal/platform/db/migrations/`.
- Never commit secrets; local defaults belong in `.env.example` / `docker-compose.yml`.
- Do not commit proprietary third-party artefacts - see "What is not in this
  repository" in the README.

## Issues and pull requests

Open an issue first for anything non-trivial; then a topic branch, a PR that closes
the issue, and a green run before merging.
