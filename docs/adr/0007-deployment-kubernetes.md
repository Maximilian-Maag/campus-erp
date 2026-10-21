# ADR-0007 — Deployment target: Kubernetes

- Status: Accepted
- Date: 2026-10-20

## Context

Deployment target is a Kubernetes cluster. Load: 15k users per university, peak =
all users concurrently, a few hundred by day, 10-20 at night (batch window). The
system is stateful (Postgres, object storage) and has bursty batch work (ETL,
statements, statistics, PDF generation).

## Decision

Deploy on Kubernetes with:

- **CloudNativePG** for PostgreSQL — one cluster, **one database per tenant**
  (ADR-0002), primary + read replicas.
- **PgBouncer** (transaction pooling) — one pool per tenant.
- Stateless Deployments: `web` (Angular assets), `api`, `worker`, `connectors`,
  `pdf` (Gotenberg) — each horizontally scalable via HPA.
- **River** (Postgres-backed queue) for async work — no extra broker to operate.
- **Scheduler**: k8s CronJob (nightly batch, dunning, statement fetch).
- **Object storage**: S3-compatible (MinIO on-prem or university S3).
- **Redis** for sessions/rate-limiting/cache (or Postgres if we choose to avoid it).
- **Ingress** (nginx/Traefik) + cert-manager; per-tenant hostnames.
- **SFTP file gateway** for file-based interfaces (LBV, statistics).
- **Observability**: OpenTelemetry Collector + Prometheus + Grafana + Loki.

## Consequences

- Read replicas + aggressive catalogue caching are the mechanism that absorbs the
  "everyone at once" peak; writes stay on the primary.
- River removes a broker from the ops surface (consistent with "simple to deploy").
- Helm charts and CloudNativePG config live in `/deploy`.
- Node count/pooling must be sized for the peak, not the average.

## Alternatives considered

- **VMs + systemd.** Simpler per node, but the target is explicitly k8s and the
  scaling story for the peak is weaker. Rejected.
- **Managed cloud services.** Out of scope: universities typically run on-prem.
