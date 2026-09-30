FROM node:24-alpine3.23 AS frontend
WORKDIR /src/clients/web/frontend
RUN corepack enable
COPY clients/web/frontend/package.json clients/web/frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY clients/web/frontend/ ./
RUN pnpm build:backend

# ============================================================
# Stage 1: Build the clawy binary
# ============================================================
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git make bash

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
COPY --from=frontend /src/clients/web/backend/dist ./clients/web/backend/dist
RUN SKIP_WEB_BUILD=1 make build

# ============================================================
# Stage 2: Minimal runtime image
# ============================================================
FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata curl

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q --spider http://localhost:18800/api/auth/status || exit 1

# Copy binary and first-run entrypoint (same as release image).
COPY --from=builder /src/build/clawy /usr/local/bin/clawy
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 18800
ENTRYPOINT ["/entrypoint.sh"]
CMD ["start", "--public", "--no-browser"]
