# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

# ---- build stage ----
FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS builder

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
FROM debian:bookworm-slim@sha256:734a72b3d6142a71993b4bc580ca01d8ca687f77a2c422d96a961d3376b179bc

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
