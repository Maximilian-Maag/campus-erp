# ADR-0008 — Testing and quality strategy (unit / integration / e2e, mutation floor 0.85)

- Status: Accepted
- Date: 2026-10-20

## Context

The maintainer mandates: unit, integration and e2e testing, and a **minimum
mutation score of 0.85**. The repo already carries policy-as-code (vendored
`tools/policy_check.py`, hook, CI). The system has two stacks (Go, Angular) and a
heavy interface surface (file formats, ETL) where correctness is legally
significant.

## Decision

- **Unit** — pure domain packages / Angular services; table-driven in Go.
- **Integration** — `testcontainers-go` boots a real Postgres (+ MinIO, + River)
  per test; the ETL/anti-corruption layer is exercised against **synthetic fixtures
  generated from the legacy schema** (no production extract available).
- **E2E** — Playwright against a deployed stack; **axe-core** accessibility
  assertions are part of e2e (BITV/WCAG is a legal requirement, per ADR-0003).
- **Contract** — generated Angular client is tested against the OpenAPI spec.
- **Mutation ≥ 0.85**, enforced as a **blocking** gate on both stacks:
  - Go: `gremlins` (or `go-mutesting`).
  - TypeScript: `Stryker`.
  - Targets are the **domain** packages/components; generated code and thin glue
    are exempted **with a stated reason**.
- The floor is encoded in code (a runner that fails below 0.85), not in prose.
- A scored target is run **three times and must report identical numbers** before a
  score is quoted (guards against flaky-test phantom scores).

## Consequences

- CI has multiple jobs: unit, integration, e2e, contract, mutation, policy. The
  mutation job is the slowest and blocks merges.
- Synthetic fixtures must be maintained as the legacy schema is understood better.
- Exemptions are visible and reasoned; a silent gap is a defect.

## Alternatives considered

- **Coverage % instead of mutation.** Coverage measures execution, not assertion
  strength; the maintainer's standing rule is mutation-based. Rejected.
- **Mutation as advisory.** Contradicts the mandate. Rejected.
