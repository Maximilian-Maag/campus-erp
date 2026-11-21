# syntax=docker/dockerfile:1
#
# Backend image — one image, three roles (modular monolith, ADR-0004).
#
# The build stage produces all three role binaries (api / worker / connectors);
# the runtime stage ships all of them and selects one via the entrypoint or the
# container command. Keeps the image count at one while the roles stay separable
# (a compose service or a k8s Deployment overrides the command).
#
# Build:
#   docker build -t campus-erp-backend:dev backend
# Run the api:
#   docker run --rm -e DATABASE_URL=... -p 8080:8080 campus-erp-backend:dev
# Run the worker:
#   docker run --rm -e DATABASE_URL=... campus-erp-backend:dev /app/worker

# ── build ────────────────────────────────────────────────────────────────────
# GO_VERSION must satisfy the `go` directive in go.mod, and the Makefile passes the
# version it reads from there (--build-arg GO_VERSION), so the image and the module
# cannot drift apart. Override it only when building this Dockerfile by hand.
ARG GO_VERSION=1.26
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# Cache the module download as its own layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static binaries (CGO off) so the runtime image can be a minimal musl base.
ENV CGO_ENABLED=0
ARG VERSION=dev
RUN set -eux; \
    for cmd in api worker connectors; do \
      go build -trimpath -ldflags "-s -w" -o "/out/$cmd" "./cmd/$cmd"; \
    done; \
    # record the version next to the binaries for `docker inspect` / ops
    printf '%s\n' "$VERSION" > /out/VERSION

# ── runtime ──────────────────────────────────────────────────────────────────
FROM alpine:3.21 AS runtime

# ca-certificates: outbound TLS (EBICS, ELSTER later). wget: the api's compose
# healthcheck (the chart probes the same routes over HTTP).
RUN apk add --no-cache ca-certificates wget \
 && addgroup -S erp && adduser -S -G erp erp

# Artefact directories. Both roles that store bytes read a directory from the
# environment (api: ERP_DOCUMENTS_DIR, worker: ERP_OUTBOX_DIR) and refuse to start
# without one, so the image carries a writable default - mounted as a volume by
# compose, and replaced by ERP_S3_ENDPOINT in a deployment that has object storage.
RUN mkdir -p /data/documents /data/outbox && chown -R erp:erp /data
ENV ERP_DOCUMENTS_DIR=/data/documents \
    ERP_OUTBOX_DIR=/data/outbox

COPY --from=build /out/ /app/

USER erp
WORKDIR /app
EXPOSE 8080

# No image-level HEALTHCHECK: one image serves three roles and only the api answers
# HTTP, so a probe baked in here marks worker and connectors unhealthy for ever -
# docker compose `up --wait` then never returns. The api's health is asserted where
# the role is chosen: docker-compose.yml and deploy/helm/campus-erp (api.yaml).

# CMD, not ENTRYPOINT: `command:` in compose and Kubernetes is the image's CMD and
# replaces it, whereas an ENTRYPOINT would keep `/app/api` and append the role path
# as an argument - every "role" then ran the api binary.
CMD ["/app/api"]
