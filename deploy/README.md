# Deployment

Target: Kubernetes (ADR-0007). This directory holds the Helm chart(s) and
manifests. **Placeholder** — populated as services land.

## Intended shape

```
deploy/
  helm/his-erp/            umbrella chart
    values.yaml
    templates/
      api.yaml             Deployment + Service + HPA
      worker.yaml          Deployment
      connectors.yaml      Deployment (native deps image)
      web.yaml             Deployment (Angular assets)
      scheduler.yaml       CronJob
      ingress.yaml         per-tenant hostnames
  cluster/
    cloudnativepg.yaml     Postgres cluster (database per tenant)
    minio.yaml             object storage
    redis.yaml
```

## Runtime components

| Component | Kind | Scaling |
|---|---|---|
| web | Deployment | HPA (assets) |
| api | Deployment | HPA (CPU/RPS) — the read-peak surface |
| worker | Deployment | HPA (queue depth) |
| connectors | Deployment | fixed / low (native deps) |
| pdf | Deployment | HPA (bursty) |
| scheduler | CronJob | one |
| postgres | CloudNativePG | 1 primary + N replicas |
| pgbouncer | Deployment | one pool per tenant |
| object storage | MinIO/operator | stateful |
| redis | StatefulSet | stateful |

## Not yet

No manifests are provided yet; the chart lands with the first deployable slice.
