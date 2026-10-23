# Build & deploy

Everything runs through the top-level `Makefile`:

```bash
make            # short help
make help       # every target, with the resolved tag and image names
make check-tools
```

The Makefile needs only **docker**; the Go toolchain is optional (backend builds
fall back to the `golang:1.25-alpine` image), Node/npm is required for the
frontend, and `tectonic` for the C4 PDF.

## Targets at a glance

| Group | Targets |
|---|---|
| Overview | `help`, `version`, `check-tools` |
| Everything | `all` (C4 PDF + build backend & frontend + images) |
| Quality | `test`, `quality`, `policy`, `policy-ci`, `hooks` |
| C4 model | `c4-validate`, `c4-export`, `c4-pdf`, `c4-view` |
| Backend | `backend`, `backend-build`, `backend-vet`, `backend-fmt`, `backend-tidy`, `backend-test`, `backend-test-integration`, `backend-mutation`, `backend-run-api` |
| Frontend | `frontend`, `frontend-install`, `frontend-build`, `frontend-test`, `frontend-e2e-install`, `frontend-e2e`, `frontend-mutation`, `frontend-serve` |
| Contract | `contracts-validate` |
| Images | `docker`, `docker-backend`, `docker-frontend`, `docker-images`, `docker-push` |
| Local stack | `up`, `down`, `restart`, `ps`, `logs`, `db-up`, `db-shell`, `clean-data` |
| Deploy (one host) | `deploy`, `deploy-backend`, `deploy-frontend` |
| Iterate on one half | `workon-frontend`, `workon-backend`, `dev-up` |
| Kubernetes | `helm-lint`, `helm-template`, `deploy-k8s`, `k8s-backend`, `k8s-frontend`, `k8s-status`, `k8s-rollback` |
| Clean | `clean`, `clean-images`, `distclean` |

## C4 model → PDF

```bash
make c4-validate   # Structurizr DSL syntax check
make c4-export     # c4/export/structurizr-<View>.png
make c4-pdf        # c4/export/campus-erp-c4-model.pdf
make c4-view       # interactive Structurizr Lite on http://localhost:8080
```

Pipeline (`tools/c4_pdf.sh`):

```
c4/workspace.dsl
  └─ Structurizr CLI (docker)  export -f plantuml   →  c4/export/structurizr-*.puml
       └─ PlantUML (docker)    -tpng                 →  c4/export/structurizr-*.png
            └─ tectonic        c4/pdf/template.tex   →  c4/export/campus-erp-c4-model.pdf
```

The report template is `c4/pdf/template.tex` (edit that; the generated copies in
`c4/export/` are git-ignored). `tools/c4_pdf.sh` also takes `--export-only`
(refresh PNGs) and `--no-export` (rebuild the PDF from existing PNGs).

Two pitfalls, both handled by the script:

- The `structurizr/structurizr` image runs as a **non-root** user, so it cannot
  write into a root-owned `c4/export/`; the script passes `--user $(id -u):$(id -g)`.
- That image **cannot render images locally** ("Exporting to PNG/SVG is not
  supported in this build" — the PNG/SVG path needs the hosted API via `--url`),
  so the script exports PlantUML and renders with the `plantuml/plantuml` image.
  The export has no `!include`, so it renders fully offline.

## Images

```bash
make docker-images                 # campus-erp-backend:<tag>, campus-erp-frontend:<tag>
make docker-images TAG=1.2.3
make docker-push IMAGE_BACKEND=ghcr.io/maximilian-maag/campus-erp-backend:1.2.3 \
                 IMAGE_FRONTEND=ghcr.io/maximilian-maag/campus-erp-frontend:1.2.3
```

- **backend** (`backend/Dockerfile`): multi-stage; builds `api`, `worker` and
  `connectors` from one source tree into one image. The entrypoint is `/app/api`;
  the worker/connectors roles override the command (`/app/worker`, `/app/connectors`)
  — the modular monolith (ADR-0004) as one image.
- **frontend** (`frontend/Dockerfile`): Angular production build served by nginx;
  `frontend/nginx.conf` proxies `/api` to `${API_UPSTREAM}` (same-origin API,
  ADR-0002) and does the SPA fallback.

## Local stack (docker compose)

```bash
make up            # build + start postgres, api, worker, connectors, web
make ps
make logs S=api
make db-shell
make down          # keeps the volume
make clean-data    # drops the database volume
```

Ports (overridable in `.env`, see `.env.example`): web `8081`, api `8080`,
postgres `5433`.

## Deploy — one half at a time

The point of splitting `deploy-backend` / `deploy-frontend` is that you can
deploy **one** half and keep working on the **other** against it:

```bash
# Deploy the backend, work on the frontend against it
make deploy-backend
make workon-frontend API_PROXY_TARGET=http://my-host:8080   # ng serve, /api proxied

# Deploy the frontend, work on the backend against the local DB
make deploy-frontend
make workon-backend                                         # api on :8080
```

`make up` (or `DOCKER_HOST=ssh://… make deploy`) targets a single host.

## Kubernetes (Helm)

The chart is `deploy/helm/campus-erp` (api, worker, connectors, web + optional
Ingress/HPA). Postgres is expected to be provisioned separately (CloudNativePG,
one database per tenant — ADR-0002/0007); the chart only needs a DSN.

```bash
make helm-lint                                  # helm lint
make helm-template                              # render locally (no cluster)
make deploy-k8s                                 # push images + helm upgrade --install
make k8s-backend  IMAGE_BACKEND=...:1.2.3       # roll just the api/worker
make k8s-frontend IMAGE_FRONTEND=...:1.2.3      # roll just the web tier
make k8s-status
make k8s-rollback
```

Provide the DSN either as an existing secret or inline:

```bash
helm upgrade --install campus-erp deploy/helm/campus-erp \
  --set database.existingSecret=campus-erp-dsn \
  --set ingress.enabled=true
```

## Knobs (override on the command line or in `.env`)

`TAG`, `IMAGE_BACKEND`, `IMAGE_FRONTEND`, `API_PROXY_TARGET`, `DATABASE_URL`,
`NAMESPACE`, `RELEASE`, `KUBE_CONTEXT`, `GO_IMAGE`, `STRUCTURIZR_IMAGE`,
`PLANTUML_IMAGE`, `MUTATION_THRESHOLD`.
