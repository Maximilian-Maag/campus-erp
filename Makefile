# campus-erp — one entry point for installing, building, running, testing and deploying.
#
#   make            # this help
#   make deps       # install dependencies (Go modules, npm packages)
#   make build      # backend binaries + the Angular bundle
#   make run        # build the images and start the stack (postgres, api, worker, connectors, web)
#   make stop       # stop the stack (the database volume is kept)
#   make test       # the fast quality suite
#
# Everything is driven from the repo root. Only docker is required: Go runs in the
# golang:$(GO_VERSION)-alpine image when the local toolchain is absent, Node/npm is
# needed for the frontend, and the architecture report needs tectonic. See
# docs/build-and-deploy.md and AGENTS.md for the quality floor.

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
# Registry of record and the account that may write to it. The credential is
# DOCKER_HUB_TOKEN - a Docker Hub access token with read/write, never the account
# password - and it is read from the environment only, so nothing is stored in the
# repository. `make docker-login docker-publish`.
REGISTRY      ?= docker.io/maximilianmaag
REGISTRY_USER ?= maximilianmaag
REGISTRY_HOST ?= $(firstword $(subst /, ,$(REGISTRY)))
# Further tags to publish alongside TAG (space separated), e.g. PUSH_TAGS="v1.2.3 latest".
PUSH_TAGS     ?=
# What the binary/image records as its version. It follows TAG, but a channel build
# (`TAG=latest` for dev, `TAG=staging`) must still say which commit it came from, so
# docker-publish passes the `git describe` value here instead.
BUILD_VERSION ?= $(TAG)
COMPOSE        ?= docker compose
# Read from go.mod, never hand-pinned: a `go` directive bump that does not reach the
# image is a build that fails at `go mod download` (or worse, compiles with an older
# toolchain silently). Passed to backend/Dockerfile as GO_VERSION.
GO_VERSION     ?= $(shell sed -n 's/^go \([0-9][0-9]*\.[0-9][0-9]*\).*/\1/p' $(BACKEND)/go.mod)
GO_IMAGE       ?= golang:$(GO_VERSION)-alpine
STRUCTURIZR_IMAGE ?= structurizr/structurizr:latest
STRUCTURIZR_LITE  ?= structurizr/lite:latest
PLANTUML_IMAGE    ?= plantuml/plantuml:latest
MUTATION_THRESHOLD ?= 85

# Where `workon-frontend` looks for the *other* half when it is deployed elsewhere.
API_PROXY_TARGET ?= http://localhost:8080   # workon-frontend -> deployed backend
DATABASE_URL     ?= postgres://postgres:erp@localhost:5433/campus_erp?sslmode=disable

# Kubernetes (deploy-k8s / k8s-status). Set KUBE_CONTEXT to target a specific cluster.
NAMESPACE    ?= campus-erp
RELEASE      ?= campus-erp
CHART        ?= $(DEPLOY)/helm/campus-erp
KUBE_CONTEXT ?=
KUBECTL      := kubectl $(if $(KUBE_CONTEXT),--context $(KUBE_CONTEXT),)
HELM         := helm $(if $(KUBE_CONTEXT),--kube-context $(KUBE_CONTEXT),)

# Use the local Go toolchain when present, otherwise the container. The container
# gets a named module cache volume: without one every `go` invocation downloads the
# whole module set again, and `make deps` would install into a throwaway layer.
GO_DOCKER = docker run --rm -v "$(BACKEND)":/app -v campus-erp-gomod:/go/pkg/mod -w /app \
              -e GOPATH=/go -e GOCACHE=/tmp/gocache $(GO_IMAGE) go
ifeq ($(shell command -v go >/dev/null 2>&1 && echo yes),)
GO := $(GO_DOCKER)
GO_IN_DOCKER := 1
else
GO := go
GO_IN_DOCKER := 0
endif

# Karma and Stryker need a Chrome binary. CI has one; locally take the Playwright
# chromium when CHROME_BIN is not already set, so `make test` works after `make deps`
# without the manual export docs/build-and-deploy.md used to ask for.
CHROME_BIN ?= $(lastword $(wildcard $(HOME)/.cache/ms-playwright/chromium-*/chrome-linux*/chrome))
ifneq ($(CHROME_BIN),)
export CHROME_BIN
endif

# ── helpers ─────────────────────────────────────────────────────────────────
define require
@command -v $(1) >/dev/null 2>&1 || { echo "error: '$(1)' is required for this target"; exit 1; }
endef

.PHONY: help check-tools \
        deps go-deps frontend-install build backend-build frontend-build \
        run stop logs test mutation quality clean \
        backend-vet backend-test backend-mutation \
        frontend-test frontend-e2e-install frontend-e2e frontend-mutation \
        policy policy-ci hooks test-tools contracts-check contracts-gen contracts-validate \
        docker-images docker-backend docker-frontend docker-login docker-publish \
        deploy-backend deploy-frontend helm-lint helm-template deploy-k8s k8s-status \
        db-up db-shell clean-data workon-backend workon-frontend c4-pdf

help: ## Show this help
	@echo "campus-erp — deps, build, run, test"
	@awk 'BEGIN{FS=":.*## "} /^##@@ /{printf "\n%s\n", substr($$0,6); next} \
	      /^[a-zA-Z0-9_.-]+:.*## /{printf "  \033[36m%-24s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo
	@echo "vars: TAG=$(TAG)  IMAGE_BACKEND=$(IMAGE_BACKEND)  IMAGE_FRONTEND=$(IMAGE_FRONTEND)"
	@echo "      registry=$(REGISTRY)  go=$(if $(filter 1,$(GO_IN_DOCKER)),docker,$(shell command -v go))"

check-tools: ## Report which optional tools are available
	@for t in docker go node npm npx helm kubectl python3 tectonic gremlins; do \
	  if command -v $$t >/dev/null 2>&1; then echo "  ok      $$t"; else echo "  MISSING $$t"; fi; \
	done

##@@ start here
deps: go-deps frontend-install ## Install the dependencies (Go modules, npm packages)

build: backend-build frontend-build ## Build the backend binaries and the Angular bundle

run: ## Build the images and start the stack (postgres, api, worker, connectors, web)
	$(call require,docker)
	$(COMPOSE) up -d --build
	# Wait on the api's own readiness route rather than on `--wait`: a cold postgres
	# needs a while to accept connections, and `--wait` aborts the moment a container
	# is *unhealthy* once - which a first boot with migrations can be, permanently red
	# for a stack that was about to come up fine.
	@echo "waiting for the api (/readyz)"
	@for i in $$(seq 1 60); do \
	  if curl -fsS -o /dev/null "http://localhost:$${API_PORT:-8080}/readyz" 2>/dev/null; then \
	    echo "ok: http://localhost:$${API_PORT:-8080}/readyz"; break; \
	  fi; \
	  if (( i == 60 )); then \
	    $(COMPOSE) ps; $(COMPOSE) logs --tail=30 api; \
	    echo "error: the api did not report ready within 2 minutes"; exit 1; \
	  fi; \
	  sleep 2; \
	done
	@echo
	@echo "web  http://localhost:8081"
	@echo "api  http://localhost:8080  (readiness: /readyz)"

stop: ## Stop the stack (keeps the database volume)
	$(COMPOSE) down

logs: ## Tail the stack's logs (make logs S=api to filter)
	$(COMPOSE) logs -f $(S)

test: ## Fast suite: Go, Angular, tooling, contract (no mutation)
	@$(ROOT)/tools/run_tests.sh

mutation: backend-mutation frontend-mutation ## Mutation gates: gremlins per package + Stryker (floor 85)

quality: ## Everything: policy, contract, the fast suite and the mutation gates
	@$(MAKE) --no-print-directory policy contracts-check test mutation

clean: ## Remove build artefacts (bin, dist, c4 export)
	rm -rf $(BACKEND)/bin $(FRONTEND)/dist $(FRONTEND)/.angular \
	       $(C4)/export $(ROOT)/build

##@@ components (the verbs above compose these; CI runs them directly)
needs_npm = @if [[ ! -d $(FRONTEND)/node_modules ]]; then echo "error: run 'make deps' first (no $(FRONTEND)/node_modules)"; exit 1; fi

go-deps: ## Download the Go modules
	cd $(BACKEND) && $(GO) mod download

backend-build: ## Build api/worker/connectors into backend/bin/
	cd $(BACKEND) && mkdir -p bin && \
	  for cmd in api worker connectors; do \
	    echo "  build $$cmd"; $(GO) build -trimpath -ldflags "-s -w" -o "bin/$$cmd" "./cmd/$$cmd"; \
	  done

backend-vet: ## go vet
	cd $(BACKEND) && $(GO) vet ./...

backend-test: ## Go unit + integration tests (integration needs DATABASE_URL)
	cd $(BACKEND) && $(GO) test ./...

# One package per gremlins run, each with its own cache dir: a shared cache yields
# wrong, non-reproducible scores (ADR-0008). PKG=<path> runs a single package, which
# is how CI shards the sweep; without it every target in backend/mutation.json runs.
# Output is streamed through tee: a shard is minutes long and a silent job looks hung.
backend-mutation: ## gremlins (floor 85 per package; PKG=<path> for one)
	$(call require,gremlins)
	@if [[ -n "$(PKG)" ]]; then targets="$(PKG)"; else \
	  targets=$$(python3 -c "import json;print('\n'.join(t['path'] for t in json.load(open('$(BACKEND)/mutation.json'))['targets']))"); \
	fi; \
	cache=$$(mktemp -d); trap 'rm -rf "$$cache"' EXIT; \
	for pkg in $$targets; do \
	  echo "== mutation/go $$pkg"; \
	  log="$$cache/$${pkg//\//_}.log"; \
	  if ! ( cd $(BACKEND) && XDG_CACHE_HOME="$$cache/$${pkg//\//_}" \
	         stdbuf -oL -eL gremlins unleash --threshold-efficacy $(MUTATION_THRESHOLD) "./$$pkg" 2>&1 | tee "$$log" ); then \
	    echo "FAIL mutation/go $$pkg (below floor, or the run itself failed)"; exit 1; \
	  fi; \
	  if grep -q "No results to report" "$$log"; then echo "FAIL mutation/go $$pkg: vacuous run"; exit 1; fi; \
	  echo "PASS mutation/go $$pkg"; \
	done

frontend-install: ## npm ci
	cd $(FRONTEND) && npm ci

frontend-build: ## Production build (frontend/dist/)
	$(needs_npm)
	cd $(FRONTEND) && npm run build -- --configuration production

frontend-test: ## Karma unit tests (headless) and the two i18n guards
	$(needs_npm)
	cd $(FRONTEND) && npm run i18n
	cd $(FRONTEND) && npm run test:ci

frontend-e2e-install: ## Install the Playwright browsers
	$(needs_npm)
	cd $(FRONTEND) && npx playwright install --with-deps chromium

frontend-e2e: ## Playwright + axe end-to-end tests
	$(needs_npm)
	cd $(FRONTEND) && npm run e2e

frontend-mutation: ## Stryker mutation (floor 85)
	$(needs_npm)
	cd $(FRONTEND) && npm run mutation

##@@ policy, contract, tooling
policy: ## Run the policy checker (changed files)
	@python3 $(ROOT)/tools/policy_check.py --changed

policy-ci: ## Run the policy checker in CI mode (all files)
	@python3 $(ROOT)/tools/policy_check.py --ci

hooks: ## Enable the repo's git hooks (core.hooksPath)
	@git -C $(ROOT) config core.hooksPath .githooks
	@echo "hooks enabled"

test-tools: ## Self-test the tooling (ER-model generator + API contract + domain codes)
	@python3 $(ROOT)/tools/test_erm.py
	@python3 $(ROOT)/tools/api_contract.py selftest
	@python3 $(ROOT)/tools/test_domaincodes.py

contracts-check: ## Verify spec, client and domain codes match the Go routes (drift guard)
	@python3 $(ROOT)/tools/api_contract.py selftest
	@python3 $(ROOT)/tools/api_contract.py check
	@python3 $(ROOT)/tools/gen_openapi.py --selftest
	@python3 $(ROOT)/tools/gen_openapi.py --check
	@python3 $(ROOT)/tools/gen_domaincodes.py --check

contracts-gen: ## Regenerate contracts/openapi.yaml, the Angular client and the domain codes
	@python3 $(ROOT)/tools/gen_openapi.py
	@python3 $(ROOT)/tools/api_contract.py gen
	@python3 $(ROOT)/tools/gen_domaincodes.py

contracts-validate: ## Lint contracts/openapi.yaml
	@npx --yes @redocly/cli lint --config $(ROOT)/.redocly.yaml $(ROOT)/contracts/openapi.yaml

##@@ images & deploy
docker-images: docker-backend docker-frontend ## Build both container images

docker-backend: ## Build the backend image (IMAGE_BACKEND)
	$(call require,docker)
	docker build --tag "$(IMAGE_BACKEND)" --build-arg VERSION="$(BUILD_VERSION)" \
	  --build-arg GO_VERSION="$(GO_VERSION)" $(BACKEND)

docker-frontend: ## Build the frontend image (IMAGE_FRONTEND)
	$(call require,docker)
	docker build --tag "$(IMAGE_FRONTEND)" $(FRONTEND)

docker-login: ## Log in to the registry with DOCKER_HUB_TOKEN (from the environment)
	$(call require,docker)
	if [[ -z $${DOCKER_HUB_TOKEN:-} ]]; then
	  echo "error: DOCKER_HUB_TOKEN is not set (a Docker Hub access token with read/write)"
	  exit 1
	fi
	# Through the environment, not argv: a token in the command line is visible to
	# every process on the machine.
	printenv DOCKER_HUB_TOKEN | docker login --username "$(REGISTRY_USER)" --password-stdin "$(REGISTRY_HOST)"

docker-publish: ## Build both images and push them to $(REGISTRY)
	$(call require,docker)
	# Build once, retag for publication: a second build would only re-push the same
	# bytes under a different name (and could differ if the tree moved on).
	$(MAKE) --no-print-directory docker-images \
	  BUILD_VERSION="$(shell git -C $(ROOT) describe --tags --always --dirty 2>/dev/null || echo dev)" \
	  IMAGE_BACKEND="$(REGISTRY)/campus-erp-backend:$(TAG)" \
	  IMAGE_FRONTEND="$(REGISTRY)/campus-erp-frontend:$(TAG)"
	for image in campus-erp-backend campus-erp-frontend; do
	  for tag in $(TAG) $(PUSH_TAGS); do
	    if [[ $$tag != "$(TAG)" ]]; then
	      docker tag "$(REGISTRY)/$$image:$(TAG)" "$(REGISTRY)/$$image:$$tag"
	    fi
	    docker push "$(REGISTRY)/$$image:$$tag"
	  done
	done

deploy-backend: docker-backend ## Build + (re)start the backend services only
	$(COMPOSE) up -d --no-deps postgres api worker connectors

deploy-frontend: docker-frontend ## Build + (re)start the web service only
	$(COMPOSE) up -d --no-deps web

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

deploy-k8s: docker-publish ## helm upgrade --install the whole release
	$(call require,helm)
	$(HELM) upgrade --install $(RELEASE) $(CHART) \
	  --namespace $(NAMESPACE) --create-namespace \
	  --set images.backend.repository="$(firstword $(subst :, ,$(IMAGE_BACKEND)))" \
	  --set images.backend.tag="$(lastword $(subst :, ,$(IMAGE_BACKEND)))" \
	  --set images.frontend.repository="$(firstword $(subst :, ,$(IMAGE_FRONTEND)))" \
	  --set images.frontend.tag="$(lastword $(subst :, ,$(IMAGE_FRONTEND)))"

k8s-status: ## Show pods/services/ingress
	$(call require,kubectl)
	$(KUBECTL) -n $(NAMESPACE) get deploy,svc,ing,po

##@@ database & one half at a time
db-up: ## Start only Postgres (for local backend/integration tests)
	$(COMPOSE) up -d postgres

db-shell: ## psql into the running Postgres
	$(COMPOSE) exec postgres psql -U postgres -d $${POSTGRES_DB:-campus_erp}

clean-data: ## Stop the stack and delete the database volume
	$(COMPOSE) down -v

workon-backend: db-up ## Run the api locally against the compose database
	@echo "backend: http://localhost:8080  ->  $(DATABASE_URL)"
	cd $(BACKEND) && DATABASE_URL="$(DATABASE_URL)" $(GO) run ./cmd/api

workon-frontend: ## Dev-serve the SPA, proxying /api to API_PROXY_TARGET (deployed backend)
	@echo "frontend: http://localhost:4200  ->  /api -> $(API_PROXY_TARGET)"
	cd $(FRONTEND) && API_PROXY_TARGET="$(API_PROXY_TARGET)" npm start -- --proxy-config proxy.conf.mjs

##@@ report
c4-pdf: ## Build the architecture report PDF (C4 + ER models; needs docker + tectonic)
	$(call require,docker)
	tools/c4_pdf.sh
