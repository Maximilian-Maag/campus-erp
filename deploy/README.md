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
      NOTES.txt
```

Not (yet) in the chart: Postgres/CloudNativePG, PgBouncer pool registry, MinIO
object storage, Redis, the scheduler CronJob, and the `pdf` service. They land
with the slices that need them.

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
