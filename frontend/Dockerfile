# syntax=docker/dockerfile:1
#
# Frontend image — Angular SPA built once, served as static assets by nginx.
#
# The API is called on the same origin under /api (see src/app/core/tenant.ts),
# so nginx proxies /api to the backend service. That keeps the tenant as the first
# hostname label only (ADR-0002) and avoids CORS entirely.
#
# Build:
#   docker build -t campus-erp-frontend:dev frontend
# Run (standalone, backend on the host):
#   docker run --rm -p 8081:80 -e API_UPSTREAM=host.docker.internal:8080 campus-erp-frontend:dev

# ── build ────────────────────────────────────────────────────────────────────
FROM node:22-alpine AS build
WORKDIR /app

# Cache dependencies as their own layer.
COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build -- --configuration production

# ── runtime ──────────────────────────────────────────────────────────────────
FROM nginx:1.27-alpine AS runtime

# SPA + /api proxy config (templated: API_UPSTREAM is substituted at start-up).
COPY nginx.conf /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist/frontend/browser /usr/share/nginx/html

# Default upstream used when API_UPSTREAM is not set (compose service name).
ENV API_UPSTREAM=api:8080

EXPOSE 80
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1/healthz || exit 1
