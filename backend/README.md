# Backend (Go)

Modular monolith (ADR-0004), built as one binary that runs in three roles:
`api`, `worker`, `connectors` (ADR-0005).

This is a **skeleton**: it compiles and its domain packages are unit-tested, but it
does not yet implement business logic or talk to Postgres.

## Intended stack (added as features land)

| Concern | Choice | ADR |
|---|---|---|
| HTTP routing | stdlib `net/http` (Go 1.22+ patterns) or `chi` | 0003 |
| OpenAPI-first handlers | `huma` | 0003 |
| DB access | `pgx` + `sqlc` (typed Go from explicit SQL) | 0003 |
| Migrations | `goose` | 0003 |
| Job queue | `River` (Postgres-backed) | 0007 |
| Object storage | `minio-go` | 0007 |
| Native integrations | separate `connectors` image (ERiC, EBICS) | 0005 |

The skeleton deliberately depends on **stdlib only** so it builds and tests anywhere
without network access; external deps are introduced with their first real use.

## Layout

```
cmd/api          HTTP entrypoint (role: api)
cmd/worker       async jobs (role: worker)
cmd/connectors   native-dependency adapters (role: connectors)
internal/core    cross-cutting: audit, authz, workflow, catalogues, reporting, regulatory
internal/modules domain modules: personal, finance, kasse, klr
internal/platform config, telemetry, db pool registry
```

## Run

```bash
go build ./...
go test ./...

ERP_ROLE=api ERP_TENANT=demo go run ./cmd/api      # listens on :8080
curl localhost:8080/healthz
```

## Quality

- unit tests: `go test ./...`
- mutation (floor 0.85): see `mutation.json` (gremlins); wired in `.github/workflows/quality.yml`
