# syntax=docker/dockerfile:1

FROM golang:1.24-bookworm AS builder

WORKDIR /app

# Copy go mod files for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the keel binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /keel ./cmd/keel

# Runtime image
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /keel /keel

ENTRYPOINT ["/keel"]
