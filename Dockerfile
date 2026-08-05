# Multi-stage Dockerfile for Go API (Planeta Qosshy)

# Stage 1: Build binary
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install ca-certificates and git
RUN apk add --no-cache git ca-certificates

# Copy dependency manifests and download modules
COPY go.mod go.sum ./
RUN go mod download

# Copy application source code
COPY . .

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o main .

# Stage 2: Minimal runtime image
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies for TLS/HTTPS calls and timezones
RUN apk add --no-cache ca-certificates tzdata

# Copy compiled binary and HTML templates from builder
COPY --from=builder /app/main .
COPY --from=builder /app/templates ./templates

# Expose HTTP server port
EXPOSE 8080

# Entrypoint
CMD ["./main"]
