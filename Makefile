.PHONY: all build test lint proto clean

GOLANGCI_LINT = go tool -modfile tools/go.mod github.com/golangci/golangci-lint/v2/cmd/golangci-lint
BUF = go tool -modfile tools/go.mod github.com/bufbuild/buf/cmd/buf

BIN_DIR := bin

all: proto build

build:
	go build -o $(BIN_DIR)/keel ./cmd/keel
	go build -o $(BIN_DIR)/keel-provider-aws ./cmd/keel-provider-aws

proto:
	$(BUF) lint proto
	$(BUF) generate

test:
	go test -race -shuffle=on ./...

lint:
	$(GOLANGCI_LINT) run ./...

lint-fix:
	$(GOLANGCI_LINT) run ./... --fix

clean:
	rm -rf $(BIN_DIR) gen coverage.out
