# Multi-stage Dockerfile for Go API (Planeta Qosshy)

# Stage 1: Build binary
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy dependency manifests and download modules (a separate layer: re-used while
# go.mod/go.sum don't change, so a code change doesn't re-download everything)
COPY go.mod go.sum ./
RUN go mod download

# Copy application source code
COPY . .

# Build a static binary (migrations are embedded into it with go:embed)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o main .

# Stage 2: Minimal runtime image
FROM alpine:3.19

WORKDIR /app

# ca-certificates for HTTPS calls, tzdata for time zones;
# a user without password, home or shell: the app needs nothing more.
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -s /sbin/nologin -u 10001 app

COPY --from=builder /app/main .
COPY --from=builder /app/templates ./templates

# Product photos (a volume in docker-compose). Created here so the empty volume
# takes over this owner and the non-root app can write into it.
RUN mkdir -p /app/uploads && chown app /app/uploads

# Not root: if someone finds a hole in the app, they get a user that can't do much.
USER app

ENV GIN_MODE=release
EXPOSE 8080

# Docker marks the container "unhealthy" when /healthz stops answering.
HEALTHCHECK --interval=15s --timeout=3s --start-period=30s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

CMD ["./main"]
