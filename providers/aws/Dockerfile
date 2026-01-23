# syntax=docker/dockerfile:1

FROM golang:1.24-bookworm AS builder

WORKDIR /app

# Copy go mod files from root for replace directive
COPY go.mod go.sum ./
COPY gen/ ./gen/
COPY pkg/ ./pkg/

# Copy AWS provider module
COPY providers/aws/ ./providers/aws/

WORKDIR /app/providers/aws

# Download dependencies
RUN go mod download

# Build the AWS provider binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /keel-provider-aws ./cmd/keel-provider-aws

# Runtime image
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /keel-provider-aws /keel-provider-aws

EXPOSE 50051

ENTRYPOINT ["/keel-provider-aws"]
