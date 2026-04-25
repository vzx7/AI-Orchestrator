# Build stage for orchestrator
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build orchestrator
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -o orchestrator \
    -ldflags="-s -w -X main.version=${VERSION:-latest}" \
    ./cmd/server

# Build worker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -o worker \
    -ldflags="-s -w -X main.version=${VERSION:-latest}" \
    ./cmd/worker

# Final stage - orchestrator
FROM alpine:3.19 AS orchestrator

RUN apk add --no-cache ca-certificates wget

WORKDIR /app

COPY --from=builder /app/orchestrator .

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["./orchestrator"]

# Final stage - worker
FROM alpine:3.19 AS worker

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/worker .

EXPOSE 50051

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD true

ENTRYPOINT ["./worker"]