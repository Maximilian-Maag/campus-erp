# Build & deploy

Everything runs through the top-level `Makefile`:

```bash
make            # the help, grouped: start here / components / deploy
make deps       # Go modules + npm packages
make build      # backend binaries + the Angular bundle
make run        # build the images and start the stack
make stop       # stop it (the database volume is kept)
make test       # the fast quality suite
make mutation   # the mutation gates (floor 0.85)
```

The Makefile needs only **docker**; the Go toolchain is optional (it falls back to the
`golang:<go.mod version>-alpine` image with a named module cache), Node/npm is required for the
frontend, and `tectonic` for the architecture report PDF. `make check-tools` reports what is
installed.

## Targets at a glance

`make help` prints exactly this list, grouped; the **start here** block is all a contributor
needs day to day.

| Group | Targets |
|---|---|
| Start here | `deps`, `build`, `run`, `stop`, `logs`, `test`, `mutation`, `quality`, `clean` |
| Components (what the verbs compose; CI runs them directly) | `go-deps`, `backend-build`, `backend-vet`, `backend-test`, `backend-mutation` (`PKG=` for one package), `frontend-install`, `frontend-build`, `frontend-test`, `frontend-e2e-install`, `frontend-e2e`, `frontend-mutation` |
| Policy, contract, tooling | `policy`, `policy-ci`, `hooks`, `test-tools`, `contracts-check`, `contracts-gen`, `contracts-validate` |
| Images & deploy | `docker-images`, `docker-backend`, `docker-frontend`, `docker-login`, `docker-publish`, `deploy-backend`, `deploy-frontend`, `helm-lint`, `helm-template`, `deploy-k8s`, `k8s-status` |
| Database & one half at a time | `db-up`, `db-shell`, `clean-data`, `workon-backend`, `workon-frontend` |
| Report | `c4-pdf` |

`make test-tools` self-tests the *tooling*: the ER-model generator (`tools/test_erm.py` — one entity
per canonical table, with the newest tables named so a dropped one fails by name) and the API
contract tooling (`tools/api_contract.py selftest`, which CI also runs). It is **not wired into CI**:
the `quality` workflow runs the contract selftests but not `test_erm.py`, so a migration that adds or
removes a table must be followed by `make test-tools` by hand — the expected entity count lives in
`tools/test_erm.py` and is meant to be read and updated with the migration, not guessed.

## Running the quality suite locally

The gate is split (ADR-0008) so the common check is quick:

- `make test` — the **fast** suite: Go vet/build/unit+integration, the Angular
  build + Karma tests, the ER-generator self-test and the API-contract drift
  guard. Seconds when warm.
- `make mutation` — the mutation gates: gremlins per package + Stryker
  (floor 0.85). ~11 min on its own.
- `make quality` — everything: the policy check, the contract drift guard, the
  fast suite and the mutation gates.

Go needs `DATABASE_URL` (the integration/e2e tests must not skip):

```bash
export DATABASE_URL='postgres://postgres:erp@localhost:55432/campus_erp?sslmode=disable'
```

Point it at the compose database (`…@localhost:5433/campus_erp`) and stop the stack first
(`make stop`): the worker of a running stack consumes the jobs the integration tests enqueue, and
those tests then fail with jobs stuck in `retryable` while nothing is wrong with the code.

The frontend steps (Karma, Stryker) need a **Chrome** binary; `karma-chrome-launcher`
reads `CHROME_BIN`. The Makefile takes the Playwright chromium when `CHROME_BIN` is not set
(`make frontend-e2e-install` puts one there), so normally nothing has to be exported. To use
another browser, set `CHROME_BIN` yourself — the system default is qutebrowser, which Karma
cannot drive:

```bash
npx playwright install chromium   # if you have no CHROME_BIN and no Playwright browsers yet
export CHROME_BIN="$HOME/.cache/ms-playwright/chromium-*/chrome-linux64/chrome"
```

Without a browser the Angular and Stryker steps cannot run; they run in CI, which has one.

## Architecture report (C4 + ER models) → PDF

```bash
make c4-pdf        # c4/export/campus-erp-c4-model.pdf  (C4 + ER, landscape)
```

`tools/c4_pdf.sh` takes the finer flags when you are iterating on one half:
`--export-only` (refresh the PNGs), `--no-export` (rebuild the PDF from existing PNGs) and
`--no-erm` (C4 views only). `tools/erm.sh` regenerates and renders only the ER models; the
interactive Structurizr Lite view is
`docker run --rm -p 8080:8080 -v "$PWD/c4":/usr/local/structurizr structurizr/lite:latest`.

Pipeline (`tools/c4_pdf.sh`, which calls `tools/erm.sh`):

```
c4/workspace.dsl
  └─ Structurizr CLI (docker)  export -f plantuml   →  c4/export/structurizr-*.puml
       └─ PlantUML (docker)    -tpng                 →  c4/export/structurizr-*.png  ┐
migrations + schema/*.tsv                                                            │
  └─ tools/gen_erm.py          →  c4/erm/*.puml                                      ├─ tectonic → PDF
       └─ PlantUML (docker)    -tpng                 →  c4/export/{target,legacy}_*.png ┘
```

The report is **landscape A4, one diagram per page**, in two parts: *Part I*
C4 architecture model, *Part II* database ER models. The target ER model is read
directly from the SQL migrations (declared foreign keys); the legacy module ER
models are reconstructed from `schema/*.tsv` and `docs/03-schema-analysis.md`
(no foreign keys exist there — every edge is inferred).

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

The registry of record is **Docker Hub**, namespace `maximilianmaag`
(`docker.io/maximilianmaag/campus-erp-backend`, `…-frontend`). One tag per channel:

| tag | what it is | what moves it |
|---|---|---|
| `latest` | dev — whatever was just merged | every push to `dev` |
| `staging` | release candidate | every push to `staging` |
| `v1.2.3` | prod — built once, never moved | a `v*` tag |

```bash
make docker-images                 # campus-erp-backend:<tag>, campus-erp-frontend:<tag>
make docker-images TAG=1.2.3
make docker-login                  # needs DOCKER_HUB_TOKEN in the environment
make docker-publish TAG=latest     # dev
make docker-publish TAG=staging    # release candidate
make docker-publish TAG=v1.2.3     # prod
```

```bash
make docker-push IMAGE_BACKEND=docker.io/maximilianmaag/campus-erp-backend:v1.2.3 \
                 IMAGE_FRONTEND=docker.io/maximilianmaag/campus-erp-frontend:v1.2.3
```

`make docker-publish` builds both images once, then pushes them to `$(REGISTRY)` as
`$(TAG)` and every tag in `PUSH_TAGS` — the same bytes under each name. It does not
log in itself: `make docker-login` does that, reading `DOCKER_HUB_TOKEN` (a Hub
*access token* with read/write, never the account password) from the environment and
piping it to `docker login` on stdin, so the token is never a process argument. The
namespace is `$(REGISTRY_USER)` (defaults to the owner of `$(REGISTRY)`).

A channel tag says nothing about the commit, so the image records it instead:
`docker-publish` passes the `git describe` value as `BUILD_VERSION`, which is what the
backend binary and `/app/VERSION` report. `TAG=latest` is therefore a name for the
channel, never the version of the code in it.

`.github/workflows/release.yml` publishes those three channels — `dev` → `latest`,
`staging` → `staging`, a `v*` tag → that tag — inside the `release` environment, which holds
`DOCKER_HUB_TOKEN` as an environment secret, and then pulls the tag back and compares its
image ID with the build. `workflow_dispatch` takes the tag as an input, for republishing a
channel by hand. The dev and staging channels may publish while their quality run is still
in flight; tag a prod release on a commit that is green on `master`, because the workflow does
not re-run the gate.

`master` publishes nothing itself. It is the release line: features land on `dev`, a promotion
PR carries `dev` → `master`, and a `v*` tag on `master` is what prod runs — so the dev channel
follows the integration branch and a cluster only ever moves to a version somebody tagged.

**Prod pins the version tag.** `latest` and `staging` move, so a cluster that asks for one of
them with `imagePullPolicy: IfNotPresent` keeps running whatever it pulled first — pin
`images.backend.tag=v1.2.3` in prod, and use a channel tag with `pullPolicy: Always` only where
"the newest dev build" is the point.

Both images are public, so a cluster needs no pull secret for them.

- **backend** (`backend/Dockerfile`): multi-stage; builds `api`, `worker` and
  `connectors` from one source tree into one image. `CMD` is `/app/api`; compose and the
  chart override it with `/app/worker` / `/app/connectors` — the modular monolith
  (ADR-0004) as one image. The image also carries the artefact directories
  (`/data/documents`, `/data/outbox`) as `ERP_DOCUMENTS_DIR` / `ERP_OUTBOX_DIR`, because
  the api and the worker refuse to start without one.
- **frontend** (`frontend/Dockerfile`): Angular production build served by nginx;
  `frontend/nginx.conf` proxies `/api` to `${API_UPSTREAM}` (same-origin API,
  ADR-0002) and does the SPA fallback.

`quality.yml` builds both images on every push (the `images` job, build only, no push), so a
Dockerfile whose Go or Node base image drifts from `backend/go.mod` / `frontend/package.json`
fails a pull request rather than the next release. It then runs `make stack-smoke`, which starts
the stack from those exact images the way `make run` starts it, waits for `/readyz`, and checks
each service's PID 1 is its role binary.

The workflow carries no commands of its own: every job calls a target. `make mutation-targets`
prints the package list the `mutation-go` matrix fans out on, `make go-deps-retry` and
`make gremlins-install` retry the two network calls a shard makes before it does any work (a
module-proxy blip must not fail a package the change never touched), and `make stack-smoke` is
the whole boot-and-assert procedure described above. A copy of a target in YAML drifts from the
target, and then CI proves the copy.

## Local stack (docker compose)

```bash
make run           # build the images and start postgres, api, worker, connectors, web
make logs S=api
make db-shell
make stop          # keeps the volume
make clean-data    # drops the database volume
```

Ports (overridable in `.env`, see `.env.example`): web `8081`, api `8080`,
postgres `5433`. Artefacts live in the `erpdata` volume mounted at `/data`.

RustFS (an S3-compatible endpoint for the artefact store) is **opt-in**: the default store is
a directory (`docs/26`), so a contributor needs no S3 image. Start it when you want the S3
path:

```bash
docker compose --profile s3 up rustfs
```

and point the roles at it with `ERP_S3_ENDPOINT=http://localhost:9000` (plus `ERP_S3_BUCKET`,
`ERP_S3_ACCESS_KEY`, `ERP_S3_SECRET_KEY`). This is the endpoint the S3 store is verified
against.

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

`make run` (or `DOCKER_HOST=ssh://… docker compose up -d --build`) targets a single host.

## Kubernetes (Helm)

The chart is `deploy/helm/campus-erp` (api, worker, connectors, web + optional
Ingress/HPA). Postgres is expected to be provisioned separately (CloudNativePG,
one database per tenant — ADR-0002/0007); the chart only needs a DSN.

```bash
make helm-lint          # helm lint
make helm-template      # render locally (no cluster)
make deploy-k8s         # push images + helm upgrade --install
make k8s-status         # pods/services/ingress
```

Rolling one tier is `kubectl set image` against the release:
`kubectl -n campus-erp set image deployment/campus-erp-web web=<ref>`.

Provide the DSN either as an existing secret or inline:

```bash
helm upgrade --install campus-erp deploy/helm/campus-erp \
  --set database.existingSecret=campus-erp-dsn \
  --set ingress.enabled=true
```

## Knobs (override on the command line or in `.env`)

`TAG`, `BUILD_VERSION`, `IMAGE_BACKEND`, `IMAGE_FRONTEND`, `REGISTRY`, `REGISTRY_USER`,
`PUSH_TAGS`, `API_PROXY_TARGET`, `DATABASE_URL`, `NAMESPACE`, `RELEASE`, `KUBE_CONTEXT`,
`GO_IMAGE`, `STRUCTURIZR_IMAGE`, `PLANTUML_IMAGE`, `MUTATION_THRESHOLD`, `DOCKER_HUB_TOKEN`
(the credential `docker-login` reads — environment only, never written into `.env`).
