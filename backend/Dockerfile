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
FROM golang:1.25-alpine AS build
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

# ca-certificates: outbound TLS (EBICS, ELSTER later). wget: container healthcheck.
RUN apk add --no-cache ca-certificates wget \
 && addgroup -S erp && adduser -S -G erp erp

COPY --from=build /out/ /app/

USER erp
WORKDIR /app
EXPOSE 8080

# Container healthcheck hits the api's health route; harmless (and unused) when
# the same image runs as worker/connectors.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/api"]
