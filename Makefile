# Marin AI Makefile
# 🔨 Toolsmith: Standardizing development workflows

BINARY_NAME=marinai
GO_FILES=$(shell find . -name '*.go' -not -path "./vendor/*")

.PHONY: help check run build test test-all lint fmt clean

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

check: ## Run full dev-check script (deps, config, fmt, vet, short tests)
	@./scripts/dev-check.sh

run: ## Run the bot locally
	@go run main.go

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@go build -o $(BINARY_NAME) main.go

test: ## Run short tests
	@go test -short ./...

test-all: ## Run all tests
	@go test ./...

lint: ## Run go vet and check formatting
	@go vet ./...
	@test -z $$(gofmt -l .) || (echo "Formatting issues found. Run 'make fmt'"; exit 1)

fmt: ## Format all go files
	@gofmt -w .

clean: ## Remove binary and test artifacts
	@rm -f $(BINARY_NAME)
	@rm -f coverage.out
	@echo "Cleaned up."
