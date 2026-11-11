# campus-erp — one entry point for building, testing, packaging and deploying.
#
#   make            # short help
#   make help       # full target list
#   make all        # architecture report PDF + backend + frontend + container images
#   make c4-pdf     # the landscape architecture report (C4 + database ER models)
#
# Everything is driven from the repo root and is meant to work with only docker
# installed (Go runs in the golang:1.25-alpine image when the local toolchain is
# absent; Node is required for the frontend). See AGENTS.md for the quality floor.

SHELL := /bin/bash
.DEFAULT_GOAL := help
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c

ROOT      := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
BACKEND   := $(ROOT)/backend
FRONTEND  := $(ROOT)/frontend
C4        := $(ROOT)/c4
DEPLOY    := $(ROOT)/deploy

# Load .env (if present) so `make` and `docker compose` agree on names/ports.
-include $(ROOT)/.env
export

# ── configurable knobs ──────────────────────────────────────────────────────
TAG            ?= $(shell git -C $(ROOT) describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE_BACKEND  ?= campus-erp-backend:$(TAG)
IMAGE_FRONTEND ?= campus-erp-frontend:$(TAG)
COMPOSE        ?= docker compose
GO_IMAGE       ?= golang:1.25-alpine
STRUCTURIZR_IMAGE ?= structurizr/structurizr:latest
STRUCTURIZR_LITE  ?= structurizr/lite:latest
PLANTUML_IMAGE    ?= plantuml/plantuml:latest
MUTATION_THRESHOLD ?= 85

# Where `workon-*` looks for the *other* half when it is deployed elsewhere.
API_PROXY_TARGET ?= http://localhost:8080   # workon-frontend -> deployed backend
DATABASE_URL     ?= postgres://postgres:erp@localhost:5433/campus_erp?sslmode=disable

# Kubernetes (deploy-k8s / k8s-*). Set KUBE_CONTEXT to target a specific cluster.
NAMESPACE    ?= campus-erp
RELEASE      ?= campus-erp
CHART        ?= $(DEPLOY)/helm/campus-erp
KUBE_CONTEXT ?=
KUBECTL      := kubectl $(if $(KUBE_CONTEXT),--context $(KUBE_CONTEXT),)
HELM         := helm $(if $(KUBE_CONTEXT),--kube-context $(KUBE_CONTEXT),)

# Use the local Go toolchain when present, otherwise the container.
ifeq ($(shell command -v go >/dev/null 2>&1 && echo yes),)
GO := docker run --rm -v "$(BACKEND)":/app -w /app -e GOCACHE=/tmp/gocache $(GO_IMAGE) go
GO_IN_DOCKER := 1
else
GO := go
GO_IN_DOCKER := 0
endif

# ── helpers ─────────────────────────────────────────────────────────────────
define require
@command -v $(1) >/dev/null 2>&1 || { echo "error: '$(1)' is required for this target"; exit 1; }
endef

.PHONY: help version check-tools

help: ## Show this help
	@echo "campus-erp — build, test, package, deploy"
	@echo
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z0-9_.-]+:.*## /{printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo
	@echo "vars: TAG=$(TAG)  IMAGE_BACKEND=$(IMAGE_BACKEND)  IMAGE_FRONTEND=$(IMAGE_FRONTEND)"
	@echo "      go=$(if $(filter 1,$(GO_IN_DOCKER)),docker,$(shell command -v go))"

version: ## Print the resolved build tag
	@echo $(TAG)

check-tools: ## Report which optional tools are available
	@for t in docker go node npm npx tectonic helm kubectl python3; do \
	  if command -v $$t >/dev/null 2>&1; then echo "  ok      $$t"; else echo "  MISSING $$t"; fi; \
	done

# ════════════════════════════════════════════════════════════════════════════
# all / quality
# ════════════════════════════════════════════════════════════════════════════

.PHONY: all
all: c4-pdf backend frontend docker ## Architecture report PDF + build backend & frontend + images

.PHONY: test
test: ## Fast suite: Go + Angular + tooling + contract (no mutation)
	@STEPS=fast $(ROOT)/tools/run_tests.sh

.PHONY: mutation
mutation: ## Mutation gates only: gremlins per package + Stryker (floor 85)
	@STEPS=mutation $(ROOT)/tools/run_tests.sh

.PHONY: test-all
test-all: ## Full suite: the fast checks + the mutation gates
	@STEPS=fast,mutation $(ROOT)/tools/run_tests.sh

.PHONY: quality
quality: policy contracts-check test-all ## Policy check + contract + full quality suite

.PHONY: test-tools
test-tools: ## Self-test the tooling (ER-model generator + API contract + domain codes)
	@python3 $(ROOT)/tools/test_erm.py
	@python3 $(ROOT)/tools/api_contract.py selftest
	@python3 $(ROOT)/tools/test_domaincodes.py

.PHONY: policy
policy: ## Run the policy checker (changed files)
	@python3 $(ROOT)/tools/policy_check.py --changed

.PHONY: policy-ci
policy-ci: ## Run the policy checker in CI mode (all files)
	@python3 $(ROOT)/tools/policy_check.py --ci

.PHONY: hooks
hooks: ## Enable the repo's git hooks (core.hooksPath)
	@git -C $(ROOT) config core.hooksPath .githooks
	@echo "hooks enabled"

# ════════════════════════════════════════════════════════════════════════════
# C4 model + ER models  (workspace.dsl / c4/erm -> PNG -> landscape PDF)
# ════════════════════════════════════════════════════════════════════════════

.PHONY: c4-validate c4-export c4-pdf c4-pdf-no-erm c4-view erm erm-gen erm-export

c4-validate: ## Validate the Structurizr DSL
	$(call require,docker)
	chmod 644 "$(C4)/workspace.dsl"
	docker run --rm -v "$(C4)":/w -w /w $(STRUCTURIZR_IMAGE) validate -w workspace.dsl

c4-export: ## Export C4 views as PNG (c4/export/structurizr-*.png)
	$(call require,docker)
	tools/c4_pdf.sh --export-only

c4-pdf: ## Build the full architecture report PDF (C4 + ER models, landscape)
	$(call require,docker)
	tools/c4_pdf.sh

c4-pdf-no-erm: ## Build the report PDF with C4 views only
	tools/c4_pdf.sh --no-erm

c4-view: ## Serve the C4 model interactively (Structurizr Lite, :8080)
	$(call require,docker)
	docker run --rm -p 8080:8080 -v "$(C4)":/usr/local/structurizr $(STRUCTURIZR_LITE)

erm: ## Regenerate + render the database ER models (target + legacy modules)
	python3 tools/gen_erm.py --root "$(ROOT)"
	tools/erm.sh

erm-gen: ## Only (re)generate the ER PlantUML sources (c4/erm/*.puml)
	python3 tools/gen_erm.py --root "$(ROOT)"

erm-export: ## Render the ER models to PNG (c4/export/{target,legacy}_*.png)
	$(call require,docker)
	tools/erm.sh --no-gen

# ════════════════════════════════════════════════════════════════════════════
# Backend (Go)
# ════════════════════════════════════════════════════════════════════════════

.PHONY: backend backend-build backend-vet backend-fmt backend-tidy \
        backend-test backend-test-integration backend-mutation backend-run-api

backend: backend-build ## Build the backend binaries (backend/bin/)

backend-build: ## Build api/worker/connectors into backend/bin/
	cd $(BACKEND) && mkdir -p bin && \
	  for cmd in api worker connectors; do \
	    echo "  build $$cmd"; $(GO) build -trimpath -ldflags "-s -w" -o "bin/$$cmd" "./cmd/$$cmd"; \
	  done

backend-vet: ## go vet
	cd $(BACKEND) && $(GO) vet ./...

backend-fmt: ## gofmt (write)
	cd $(BACKEND) && $(GO) fmt ./...

backend-tidy: ## go mod tidy
	cd $(BACKEND) && $(GO) mod tidy

backend-test: ## Go unit + integration tests (integration needs DATABASE_URL)
	cd $(BACKEND) && $(GO) test ./...

backend-test-integration: db-up ## Go tests against the compose Postgres
	cd $(BACKEND) && DATABASE_URL="$(DATABASE_URL)" $(GO) test ./...

backend-mutation: ## gremlins per package (floor 85)
	$(call require,gremlins)
	@cache=$$(mktemp -d); trap 'rm -rf "$$cache"' EXIT; \
	targets=$$(python3 -c "import json;print('\n'.join(t['path'] for t in json.load(open('$(BACKEND)/mutation.json'))['targets']))"); \
	for pkg in $$targets; do \
	  echo "== mutation/go $$pkg"; \
	  out=$$( cd $(BACKEND) && XDG_CACHE_HOME="$$cache/$${pkg//\//_}" \
	          gremlins unleash --threshold-efficacy $(MUTATION_THRESHOLD) "./$$pkg" 2>&1 ) || { echo "$$out"; exit 1; }; \
	  echo "$$out"; \
	  if grep -q "No results to report" <<<"$$out"; then echo "vacuous run for $$pkg"; exit 1; fi; \
	done

backend-run-api: ## Run the api locally (foreground)
	cd $(BACKEND) && DATABASE_URL="$(DATABASE_URL)" $(GO) run ./cmd/api

# ════════════════════════════════════════════════════════════════════════════
# Frontend (Angular)
# ════════════════════════════════════════════════════════════════════════════

.PHONY: frontend frontend-install frontend-build frontend-test frontend-e2e \
        frontend-mutation frontend-serve frontend-e2e-install

frontend: frontend-install frontend-build ## Install deps + build the SPA

frontend-install: ## npm ci
	cd $(FRONTEND) && npm ci

frontend-build: ## Production build (frontend/dist/)
	cd $(FRONTEND) && npm run build -- --configuration production

frontend-test: ## Karma unit tests (headless)
	cd $(FRONTEND) && npm run test:ci

frontend-e2e-install: ## Install Playwright browsers
	cd $(FRONTEND) && npx playwright install --with-deps chromium

frontend-e2e: ## Playwright + axe e2e
	cd $(FRONTEND) && npm run e2e

frontend-mutation: ## Stryker mutation (floor 85)
	cd $(FRONTEND) && npm run mutation

frontend-serve: ## Dev server (ng serve) at http://localhost:4200
	cd $(FRONTEND) && npm start

# ════════════════════════════════════════════════════════════════════════════
# Contract
# ════════════════════════════════════════════════════════════════════════════

.PHONY: contracts-validate

contracts-validate: ## Lint contracts/openapi.yaml
	npx --yes @redocly/cli lint --config $(ROOT)/.redocly.yaml $(ROOT)/contracts/openapi.yaml

.PHONY: contracts-check contracts-gen

contracts-check: ## Verify the OpenAPI spec matches the Go routes (drift guard)
	@python3 $(ROOT)/tools/api_contract.py selftest
	@python3 $(ROOT)/tools/api_contract.py check
	@python3 $(ROOT)/tools/gen_domaincodes.py --check

contracts-gen: ## Regenerate contracts/openapi.yaml, the Angular client and the domain codes
	@/usr/bin/python3 $(ROOT)/tools/gen_openapi.py
	@python3 $(ROOT)/tools/api_contract.py gen
	@python3 $(ROOT)/tools/gen_domaincodes.py

# ════════════════════════════════════════════════════════════════════════════
# Container images
# ════════════════════════════════════════════════════════════════════════════

.PHONY: docker docker-backend docker-frontend docker-images docker-push

docker: docker-images ## Alias for docker-images

docker-backend: ## Build the backend image (IMAGE_BACKEND)
	$(call require,docker)
	docker build --tag "$(IMAGE_BACKEND)" --build-arg VERSION="$(TAG)" $(BACKEND)

docker-frontend: ## Build the frontend image (IMAGE_FRONTEND)
	$(call require,docker)
	docker build --tag "$(IMAGE_FRONTEND)" $(FRONTEND)

docker-images: docker-backend docker-frontend ## Build both images

docker-push: ## Push both images (set IMAGE_* to a registry ref first)
	$(call require,docker)
	docker push "$(IMAGE_BACKEND)"
	docker push "$(IMAGE_FRONTEND)"

# ════════════════════════════════════════════════════════════════════════════
# Local stack (docker compose)
# ════════════════════════════════════════════════════════════════════════════

.PHONY: up down restart ps logs db-up clean-data clean-images

up: ## Build + start the whole stack (postgres, api, worker, connectors, web)
	$(COMPOSE) up -d --build

down: ## Stop the stack (keeps the database volume)
	$(COMPOSE) down

restart: ## Restart all services
	$(COMPOSE) restart

ps: ## Show service status
	$(COMPOSE) ps

logs: ## Tail logs (make logs S=api to filter)
	$(COMPOSE) logs -f $(S)

db-up: ## Start only Postgres (for local backend/integration tests)
	$(COMPOSE) up -d postgres

db-shell: ## psql into the running Postgres
	$(COMPOSE) exec postgres psql -U postgres -d $${POSTGRES_DB:-campus_erp}

clean-data: ## Stop the stack and delete the database volume
	$(COMPOSE) down -v

# ════════════════════════════════════════════════════════════════════════════
# Deploy (single host via compose)  — deploy one half, work on the other
# ════════════════════════════════════════════════════════════════════════════

.PHONY: deploy deploy-backend deploy-frontend

deploy: deploy-backend deploy-frontend ## Deploy both halves to the compose host

deploy-backend: docker-backend ## Build + (re)start backend services only
	$(COMPOSE) up -d --no-deps postgres api worker connectors

deploy-frontend: docker-frontend ## Build + (re)start the web service only
	$(COMPOSE) up -d --no-deps web

# ════════════════════════════════════════════════════════════════════════════
# Work on one half against the deployed other half
# ════════════════════════════════════════════════════════════════════════════

.PHONY: workon-frontend workon-backend dev-up

workon-frontend: ## Dev-serve the SPA, proxying /api to API_PROXY_TARGET (deployed backend)
	@echo "frontend: http://localhost:4200  ->  /api -> $(API_PROXY_TARGET)"
	cd $(FRONTEND) && API_PROXY_TARGET="$(API_PROXY_TARGET)" npm start -- --proxy-config proxy.conf.mjs

workon-backend: db-up ## Run the api locally against the compose database
	@echo "backend: http://localhost:8080  ->  $(DATABASE_URL)"
	cd $(BACKEND) && DATABASE_URL="$(DATABASE_URL)" $(GO) run ./cmd/api

dev-up: db-up ## Minimal dev bootstrap (Postgres only); then workon-backend + workon-frontend

# ════════════════════════════════════════════════════════════════════════════
# Kubernetes (Helm)  — production target, ADR-0007
# ════════════════════════════════════════════════════════════════════════════

.PHONY: helm-lint helm-template deploy-k8s k8s-backend k8s-frontend k8s-status k8s-rollback

helm-lint: ## helm lint the chart
	$(call require,helm)
	helm lint $(CHART)

helm-template: ## Render the chart locally (no cluster needed)
	$(call require,helm)
	helm template $(RELEASE) $(CHART) --namespace $(NAMESPACE) \
	  --set images.backend.repository="$(firstword $(subst :, ,$(IMAGE_BACKEND)))" \
	  --set images.backend.tag="$(lastword $(subst :, ,$(IMAGE_BACKEND)))" \
	  --set images.frontend.repository="$(firstword $(subst :, ,$(IMAGE_FRONTEND)))" \
	  --set images.frontend.tag="$(lastword $(subst :, ,$(IMAGE_FRONTEND)))"

deploy-k8s: docker-push ## helm upgrade --install the whole release
	$(call require,helm)
	$(HELM) upgrade --install $(RELEASE) $(CHART) \
	  --namespace $(NAMESPACE) --create-namespace \
	  --set images.backend.repository="$(firstword $(subst :, ,$(IMAGE_BACKEND)))" \
	  --set images.backend.tag="$(lastword $(subst :, ,$(IMAGE_BACKEND)))" \
	  --set images.frontend.repository="$(firstword $(subst :, ,$(IMAGE_FRONTEND)))" \
	  --set images.frontend.tag="$(lastword $(subst :, ,$(IMAGE_FRONTEND)))"

k8s-backend: ## Roll the backend deployments to the current image
	$(call require,kubectl)
	$(KUBECTL) -n $(NAMESPACE) set image deployment/$(RELEASE)-api     api="$(IMAGE_BACKEND)"
	$(KUBECTL) -n $(NAMESPACE) set image deployment/$(RELEASE)-worker  worker="$(IMAGE_BACKEND)"
	$(KUBECTL) -n $(NAMESPACE) rollout status deployment/$(RELEASE)-api

k8s-frontend: ## Roll the web deployment to the current image
	$(call require,kubectl)
	$(KUBECTL) -n $(NAMESPACE) set image deployment/$(RELEASE)-web web="$(IMAGE_FRONTEND)"
	$(KUBECTL) -n $(NAMESPACE) rollout status deployment/$(RELEASE)-web

k8s-status: ## Show pods/services/ingress
	$(call require,kubectl)
	$(KUBECTL) -n $(NAMESPACE) get deploy,svc,ing,po

k8s-rollback: ## Roll back the last helm release
	$(call require,helm)
	$(HELM) rollback $(RELEASE) -n $(NAMESPACE)

# ════════════════════════════════════════════════════════════════════════════
# Clean
# ════════════════════════════════════════════════════════════════════════════

.PHONY: clean distclean

clean: ## Remove build artefacts (bin, dist, c4 export)
	rm -rf $(BACKEND)/bin $(FRONTEND)/dist $(FRONTEND)/.angular \
	       $(C4)/export $(ROOT)/build

clean-images: ## Remove the built container images
	-docker image rm "$(IMAGE_BACKEND)" "$(IMAGE_FRONTEND)"

distclean: clean clean-images down ## clean + clean-images + stop compose
