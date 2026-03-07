PROJECT_NAME := sandboxmatrix
BINARY_NAME := smx
MODULE := github.com/hg-dendi/sandboxmatrix

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -s -w \
	-X $(MODULE)/internal/cli.Version=$(VERSION) \
	-X $(MODULE)/internal/cli.Commit=$(COMMIT) \
	-X $(MODULE)/internal/cli.BuildDate=$(BUILD_DATE)

GO := go
GOFLAGS := -trimpath
GOLANGCI_LINT := golangci-lint

.PHONY: all build install test test-race lint fmt vet clean e2e help

all: lint test build

build:
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o bin/$(BINARY_NAME) ./cmd/smx

install:
	$(GO) install $(GOFLAGS) -ldflags '$(LDFLAGS)' ./cmd/smx

test:
	$(GO) test ./... -count=1

test-race:
	$(GO) test ./... -race -count=1

test-cover:
	$(GO) test ./... -coverprofile=coverage.out -count=1
	$(GO) tool cover -html=coverage.out -o coverage.html

lint:
	$(GOLANGCI_LINT) run ./...

fmt:
	$(GO) fmt ./...
	goimports -w .

vet:
	$(GO) vet ./...

e2e:
	bash test/e2e_test.sh

clean:
	rm -rf bin/ dist/ coverage.out coverage.html

help:
	@echo "Targets:"
	@echo "  build       Build the smx binary"
	@echo "  install     Install smx to GOPATH/bin"
	@echo "  test        Run tests"
	@echo "  test-race   Run tests with race detector"
	@echo "  test-cover  Run tests with coverage"
	@echo "  lint        Run golangci-lint"
	@echo "  fmt         Format code"
	@echo "  vet         Run go vet"
	@echo "  clean       Remove build artifacts"
