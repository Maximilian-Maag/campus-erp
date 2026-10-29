# Deployment

Target: Kubernetes (ADR-0007). This directory holds the Helm chart. Postgres is
expected to be provisioned by an operator (CloudNativePG, **one database per
tenant** — ADR-0002); the chart only consumes a DSN.

Everything here is driven by the top-level `Makefile` — see
`docs/build-and-deploy.md` for the full target list.

## Layout

```
deploy/
  helm/campus-erp/
    Chart.yaml
    values.yaml
    templates/
      _helpers.tpl
      api.yaml            Deployment + Service (Go, role=api)
      worker.yaml         Deployment (Go, role=worker)
      connectors.yaml     Deployment (Go, role=connectors)
      web.yaml            Deployment + Service (Angular/nginx)
      ingress.yaml        per-tenant hostnames (optional)
      hpa.yaml            autoscaling for api/web (optional)
      secret.yaml         chart-managed DSN (only when asked)
      postgres-cluster.yaml  CloudNativePG Cluster (gated: postgres.enabled)
      pgbouncer-pooler.yaml  PgBouncer Pooler (gated: postgres.pooler.enabled)
      scheduler.yaml      nightly batch CronJob (gated: scheduler.enabled)
      redis.yaml          Redis Deployment + Service (gated: redis.enabled)
      NOTES.txt
```

Not (yet) in the chart: the `pdf` service (Gotenberg) and the SFTP file gateway.
They land with the slices that need them.

## ADR-0007 infrastructure

The chart can provision the ADR-0007 infrastructure, all **off by default** so a
first install stays lean and each piece can instead point at a managed/on-prem
equivalent:

| value | provides |
|---|---|
| `postgres.enabled` | a CloudNativePG `Cluster` (1 primary + N replicas; one database per tenant — ADR-0002) |
| `postgres.pooler.enabled` | a CloudNativePG `Pooler` (PgBouncer, transaction pooling) |
| `scheduler.enabled` | a nightly batch `CronJob` (ETL, dunning, statement fetch) |
| `redis.enabled` | a `redis` Deployment + Service (sessions / rate-limit / cache) |
| `objectStorage.{endpoint,bucket}` | S3-compatible config consumed by the roles as the features land |

`docker-compose.yml` carries the same pieces (postgres, MinIO, Redis) for local and
single-host installs, so the dev stack matches the production shape. Async work is
River (Postgres-backed, ADR-0007) — no extra broker to operate. Still external to
the chart: OpenTelemetry/Prometheus/Grafana/Loki and the SFTP gateway, which are
cluster add-ons rather than per-release objects.

## Use

```bash
make helm-lint                          # lint
make helm-template                      # render locally, no cluster
make deploy-k8s                         # push images + helm upgrade --install
make k8s-backend  IMAGE_BACKEND=...:1.2.3   # roll only the api/worker
make k8s-frontend IMAGE_FRONTEND=...:1.2.3  # roll only the web tier
```

Directly with helm:

```bash
helm upgrade --install campus-erp deploy/helm/campus-erp \
  --namespace campus-erp --create-namespace \
  --set images.backend.repository=ghcr.io/maximilian-maag/campus-erp-backend \
  --set images.backend.tag=0.6.1 \
  --set images.frontend.repository=ghcr.io/maximilian-maag/campus-erp-frontend \
  --set images.frontend.tag=0.6.1 \
  --set database.existingSecret=campus-erp-dsn \
  --set ingress.enabled=true
```

`database.existingSecret` is recommended for production (an external secrets
operator keeps the per-tenant DSN fresh). For a first install you can set
`--set database.url=postgres://…` and the chart creates the Secret itself.

## Platform configuration

The api is a single-tenant or multi-tenant front and, with OIDC configured, the
policy-enforcing HTTP surface (ADR-0002/0003). The chart wires both:

| env | source | meaning |
|---|---|---|
| `ERP_TENANT` | `config.tenant` | the default tenant name |
| `ERP_TENANT_DSN` | db Secret key `config.tenantDsnKey` (optional) | per-tenant DSN template with `{tenant}`; when absent the api runs single-tenant against `DATABASE_URL` |
| `ERP_OIDC_ISSUER` / `ERP_OIDC_AUDIENCE` / `ERP_OIDC_JWKS_URL` | `config.oidc.*` | with a JWKS URL the api requires bearer tokens on `/api/*` and enforces per-route roles; empty disables auth (local only) |

Probes: the api's **liveness** is `/healthz` (process up) and its **readiness** is
`/readyz`, which pings the database (NFR-6) so a process that cannot serve is kept
out of the Service — a database blip does not restart the process. web serves its
own `/healthz` from nginx. The same wiring is in `docker-compose.yml` for local
and single-host installs (`make up`).

## Runtime components

| Component | Kind | Image | Scaling |
|---|---|---|---|
| web | Deployment | frontend (nginx + Angular) | HPA (assets) |
| api | Deployment | backend (`/app/api`) | HPA (CPU/RPS) — the read-peak surface |
| worker | Deployment | backend (`/app/worker`) | HPA (queue depth) |
| connectors | Deployment | backend (`/app/connectors`) | fixed / low (native deps) |
| postgres | CloudNativePG | — | 1 primary + N replicas (outside the chart) |

`web` proxies `/api` to the `api` Service on the same origin (ADR-0002: the tenant
is the first hostname label, never a path segment), so no CORS is involved.
