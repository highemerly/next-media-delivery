# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

# ---- build stage ----
FROM golang:1.27.3-bookworm@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9 AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION}" \
    -o /out/media-delivery ./cmd/media-delivery

# ---- runtime stage ----
FROM debian:bookworm-slim@sha256:99c0b25c46d0214d5a2091092e7908907dcaecd6a962cde0ac700acee222b6f3

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips42 \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/media-delivery /usr/local/bin/media-delivery

# Default cache dir (override via CACHE_DIR env or k8s emptyDir mount)
RUN mkdir -p /cache

ENV HOST=0.0.0.0 \
    PORT=3000 \
    ADMIN_PORT=3001 \
    CACHE_DIR=/cache \
    LOG_LEVEL=INFO \
    GOGC=200

EXPOSE 3000
EXPOSE 3001

ENTRYPOINT ["media-delivery"]
CMD ["serve"]
