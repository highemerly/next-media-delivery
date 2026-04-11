# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.26-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /out/media-delivery ./cmd/media-delivery

# ---- runtime stage ----
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips42 \
    ca-certificates \
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
