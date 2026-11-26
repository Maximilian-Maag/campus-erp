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
# gremlins bounds each mutant's test run by the suite's own runtime times this
# coefficient. The baseline is measured on the run gremlins does first, so a cold
# build cache inflates it: at 3 a cold ledger shard gave every mutant ~3 minutes and
# a warm one gave 0% efficacy because 3 x 2ms is shorter than process startup. The
# target warms the build cache before gremlins, which makes the baseline the real
# test time; 20 leaves room for a normal mutant and still bounds an infinite loop.
MUTATION_TIMEOUT_COEFFICIENT ?= 20
# Wall-clock bound for one shard. Empty locally: a laptop may take as long as it takes.
# CI sets it so a wedged mutant fails in minutes and is rerun, instead of sitting until
# the job timeout and being reported as a shutdown of the pull request (#186).
MUTATION_SHARD_TIMEOUT ?=

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
# A `go` on PATH is not proof of a working Go: a version-manager shim (mise, asdf) that
# has no version selected exists, answers `command -v`, and then fails on every call with
# "No version is set for shim". Probe the toolchain itself, so a broken shim falls back to
# the container instead of turning every backend target into a spurious build error.
GO_VERSION_OK := $(shell go version >/dev/null 2>&1 && echo yes)
ifeq ($(GO_VERSION_OK),)
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
        policy policy-ci hooks test-tools language language-gen \
        contracts-check contracts-gen contracts-validate \
        docker-images docker-backend docker-frontend docker-login docker-publish \
        deploy-backend deploy-frontend helm-lint helm-template deploy-k8s k8s-status \
        db-up db-shell clean-data workon-backend workon-frontend c4-pdf \
        mutation-targets go-deps-retry gremlins-install stack-smoke

help: ## Show this help
	@echo "campus-erp — deps, build, run, test"
	@awk 'BEGIN{FS=":.*## "} /^##@@ /{printf "\n%s\n", substr($$0,6); next} \
	      /^[a-zA-Z0-9_.-]+:.*## /{printf "  \033[36m%-24s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo
	@echo "vars: TAG=$(TAG)  IMAGE_BACKEND=$(IMAGE_BACKEND)  IMAGE_FRONTEND=$(IMAGE_FRONTEND)"
	@echo "      registry=$(REGISTRY)  go=$(if $(filter 1,$(GO_IN_DOCKER)),docker (local toolchain absent or not runnable),$(shell command -v go))"

check-tools: ## Report which optional tools are available
	# `go` is probed by running it: a version-manager shim answers `command -v` and then
	# fails, which used to be reported as "ok go" right before every target broke.
	@for t in docker node npm npx helm kubectl python3 tectonic gremlins; do \
	  if command -v $$t >/dev/null 2>&1; then echo "  ok      $$t"; else echo "  MISSING $$t"; fi; \
	done
	@if [[ "$(GO_IN_DOCKER)" == "1" ]]; then \
	  echo "  notice  go       not runnable locally; backend targets run it in $(GO_IMAGE)"; \
	else echo "  ok      go       $(shell go version)"; fi

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

quality: ## Everything: policy, language, contract, the fast tests and the mutation gates
	@$(MAKE) --no-print-directory policy language contracts-check test mutation

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
# Output goes to a log file, not through tee: when a shard's timeout kills gremlins,
# tee can keep waiting on the test processes it left behind, so the step never ends.
# The log is printed when the shard fails.
backend-mutation: ## gremlins (floor 85 per package; PKG=<path> for one)
	$(call require,gremlins)
	@if [[ -n "$(PKG)" ]]; then targets="$(PKG)"; else \
	  targets=$$(python3 -c "import json;print('\n'.join(t['path'] for t in json.load(open('$(BACKEND)/mutation.json'))['targets']))"); \
	fi; \
	cache=$$(mktemp -d); trap 'rm -rf "$$cache"' EXIT; \
	for pkg in $$targets; do \
	  echo "== mutation/go $$pkg"; \
	  log="$$cache/$${pkg//\//_}.log"; \
	  # Warm the build cache first: gremlins measures its timeout from the first run, \
	  # and a cold build would make that baseline the build time rather than the test time. \
	  if ! ( cd $(BACKEND) && $(GO) test -run '^$$' "./$$pkg" >/dev/null 2>&1 ); then \
	    echo "FAIL mutation/go $$pkg (the package does not build)"; exit 1; \
	  fi; \
	  if ! ( cd $(BACKEND) && XDG_CACHE_HOME="$$cache/$${pkg//\//_}" \
	         $(if $(MUTATION_SHARD_TIMEOUT),timeout --kill-after=60s $(MUTATION_SHARD_TIMEOUT)) \
	         stdbuf -oL -eL gremlins unleash --threshold-efficacy $(MUTATION_THRESHOLD) \
	           --timeout-coefficient $(MUTATION_TIMEOUT_COEFFICIENT) "./$$pkg" >"$$log" 2>&1 ); then \
	    echo "FAIL mutation/go $$pkg (below floor, or the run itself failed)"; tail -40 "$$log"; exit 1; \
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
	# Stryker has no wall-clock bound of its own, so a wedged run would sit until the
	# job timeout and surface as a red check on the pull request (#186). The same
	# variable the backend target uses bounds it; empty locally, so a laptop run is
	# never cut short.
	cd $(FRONTEND) && $(if $(MUTATION_SHARD_TIMEOUT),timeout --kill-after=60s $(MUTATION_SHARD_TIMEOUT)) npm run mutation

##@@ policy, contract, tooling
policy: ## Run the policy checker (changed files)
	@python3 $(ROOT)/tools/policy_check.py --changed

policy-ci: ## Run the policy checker in CI mode (all files)
	@python3 $(ROOT)/tools/policy_check.py --ci

language: ## Enforce the ubiquitous language and keep its page current
	@python3 $(ROOT)/tools/gen_language.py --check
	@python3 $(ROOT)/tools/check_language.py

language-gen: ## Regenerate docs/ubiquitous-language.md from the JSON
	@python3 $(ROOT)/tools/gen_language.py

hooks: ## Enable the repo's git hooks (core.hooksPath)
	@git -C $(ROOT) config core.hooksPath .githooks
	@echo "hooks enabled"

test-tools: ## Self-test the tooling (ER model + API contract + domain codes + language)
	@python3 $(ROOT)/tools/test_erm.py
	@python3 $(ROOT)/tools/api_contract.py selftest
	@python3 $(ROOT)/tools/test_domaincodes.py
	@python3 $(ROOT)/tools/test_language.py
	@python3 $(ROOT)/tools/test_mutation_target.py

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

##@@ continuous integration (what the workflow calls, rather than copying)
# These four exist because the workflow used to carry their commands. A hand-written copy of a
# target drifts from it, and then CI proves the copy: `stack-smoke` is the one that matters —
# it boots the images `docker-images` built and asserts each service is the role the chart
# asked for. The other three are the two network calls a shard makes before it does any work
# (a proxy blip must not fail a package that is nowhere near the change) and the matrix input.
mutation-targets: ## Print the mutation package list as JSON (the CI matrix reads it)
	@python3 -c "import json;print(json.dumps([t['path'] for t in json.load(open('$(BACKEND)/mutation.json'))['targets']]))"

go-deps-retry: ## `go-deps`, three attempts (CI: a module proxy blip is not a red shard)
	@for attempt in 1 2 3; do \
	  if $(MAKE) --no-print-directory -s go-deps; then echo "modules downloaded (attempt $$attempt)"; exit 0; fi; \
	  echo "go mod download failed (attempt $$attempt) - retrying"; sleep 10; \
	done; \
	echo "error: the module download still fails after 3 attempts"; exit 1

gremlins-install: ## Install gremlins into the Go bin directory, three attempts (CI)
	@if [[ -z $(GO_VERSION_OK) ]]; then echo "error: gremlins-install needs a local Go"; exit 1; fi
	@for attempt in 1 2 3; do \
	  if GOBIN=$$(go env GOPATH)/bin go install github.com/go-gremlins/gremlins/cmd/gremlins@latest; then \
	    echo "gremlins installed (attempt $$attempt)"; exit 0; fi; \
	  echo "go install failed (attempt $$attempt) - retrying"; sleep 10; \
	done; \
	echo "error: gremlins still cannot be installed after 3 attempts"; exit 1

# The stack is the product's first impression, so CI proves the images rather than the tree:
# it starts what `docker-images` built, waits the way `run` waits, and then reads PID 1 of every
# role. An ENTRYPOINT with a compose `command:` looks healthy while every "role" is the api.
stack-smoke: ## Boot the built images and assert each service runs its role
	$(call require,docker)
	# Populate the volume from the image ONCE, alone: docker copies the image's /data into a
	# fresh named volume on first use, and three services triggering that copy-up at the same
	# time can leave an entry half-written ("failed to mkdir .../documents: file exists").
	docker volume create campus-erp_erpdata >/dev/null 2>&1 || true
	docker run --rm -v campus-erp_erpdata:/data --entrypoint /bin/true "$(IMAGE_BACKEND)"
	$(MAKE) --no-print-directory run
	@curl -fsS -o /dev/null "http://localhost:$${API_PORT:-8080}/readyz"
	@curl -fsS -o /dev/null "http://localhost:$${WEB_PORT:-8081}/"
	@for role in api worker connectors; do \
	  cmd=$$($(COMPOSE) exec -T $$role cat /proc/1/cmdline | tr -d '\000'); \
	  echo "$$role -> $$cmd"; \
	  if [[ $$cmd != "/app/$$role" ]]; then \
	    echo "error: the $$role service is running $$cmd"; \
	    $(COMPOSE) logs --tail=40 $$role; exit 1; \
	  fi; \
	done
	@$(COMPOSE) ps

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
