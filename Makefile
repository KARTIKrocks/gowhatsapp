GOLANGCI_LINT_VERSION := v2.12.2
GOIMPORTS_VERSION := v0.45.0

.PHONY: all setup build test test-race coverage lint lint-fix fix fmt fmt-check vet tidy bench clean ci

all: tidy fmt vet lint build test

## CI: formatting check, vet, lint, and race tests
ci: fmt-check vet lint test-race

## Install development tools (skips if already present)
setup:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	}
	@command -v goimports >/dev/null 2>&1 || { \
		echo "Installing goimports $(GOIMPORTS_VERSION)..."; \
		go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION); \
	}

## Build
build:
	@go build ./...

## Run tests
test:
	@go test ./...

## Run tests with the race detector
test-race:
	@go test -race -count=1 ./...

## Run tests with coverage and print the summary
coverage:
	@go test -race -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out | tail -1
	@echo "Full report: go tool cover -html=coverage.out"

## Lint
lint: setup
	@golangci-lint run --timeout=5m ./...

## Lint with auto-fix
lint-fix: setup
	@golangci-lint run --fix --timeout=5m ./...

## Fix formatting and lint issues
fix: fmt lint-fix

## Format
fmt: setup
	@gofmt -s -w .
	@goimports -w .

## Check formatting without modifying files (used in CI)
fmt-check: setup
	@test -z "$$(gofmt -s -l . | tee /dev/stderr)" || { echo "Unformatted files found. Run 'make fmt'."; exit 1; }
	@test -z "$$(goimports -l . | tee /dev/stderr)" || { echo "Unordered imports found. Run 'make fmt'."; exit 1; }

## Vet
vet:
	@go vet ./...

## Tidy modules
tidy:
	@go mod tidy

## Benchmarks
bench:
	@go test -bench=. -benchmem ./...

## Remove build artifacts
clean:
	@rm -f coverage*.out
	@go clean -cache -testcache
