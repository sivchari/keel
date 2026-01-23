.PHONY: all build test lint proto clean

GO := go
BUF := buf
GOLANGCI_LINT := golangci-lint

# Binary names
KEEL_BIN := keel
PROVIDER_AWS_BIN := keel-provider-aws

# Directories
BIN_DIR := bin
PROTO_DIR := proto
GEN_DIR := gen

all: proto build

## Build
build: build-keel build-provider-aws

build-keel:
	$(GO) build -o $(BIN_DIR)/$(KEEL_BIN) ./cmd/keel

build-provider-aws:
	$(GO) build -o $(BIN_DIR)/$(PROVIDER_AWS_BIN) ./cmd/keel-provider-aws

## Proto
proto: proto-lint proto-generate

proto-lint:
	$(BUF) lint $(PROTO_DIR)

proto-generate:
	$(BUF) generate

proto-breaking:
	$(BUF) breaking $(PROTO_DIR) --against '.git#branch=main'

## Test
test:
	$(GO) test -race -shuffle=on ./...

test-coverage:
	$(GO) test -race -shuffle=on -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

## Lint
lint:
	$(GOLANGCI_LINT) run ./...

## Clean
clean:
	rm -rf $(BIN_DIR)
	rm -rf $(GEN_DIR)

## Dev
dev-deps:
	$(GO) install github.com/bufbuild/buf/cmd/buf@latest
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

## Docker
docker-build:
	docker build -t keel:latest -f Dockerfile .
	docker build -t keel-provider-aws:latest -f providers/aws/Dockerfile .

## Help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all              Build everything (default)"
	@echo "  build            Build all binaries"
	@echo "  proto            Lint and generate proto files"
	@echo "  test             Run tests"
	@echo "  lint             Run linter"
	@echo "  clean            Clean build artifacts"
	@echo "  dev-deps         Install development dependencies"
	@echo "  docker-build     Build Docker images"
