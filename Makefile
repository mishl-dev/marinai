# Toolsmith 🔨 - Development Makefile
# This file standardizes common development tasks.

# Configuration
BINARY_NAME=marinai
GO=go
SCRIPT_DIR=./scripts

.PHONY: all build run test lint check clean help

all: check build

# 🏗️ Build the binary
build:
	@echo "🔨 Building $(BINARY_NAME)..."
	$(GO) build -o $(BINARY_NAME) main.go
	@echo "✅ Build complete!"

# 🚀 Run the bot
run:
	@echo "🚀 Starting $(BINARY_NAME)..."
	$(GO) run main.go

# 🧪 Run tests
test:
	@echo "🧪 Running tests..."
	$(GO) test ./...

# 🧹 Lint and Format
lint:
	@echo "🧹 Formatting and Linting..."
	$(GO) fmt ./...
	$(GO) vet ./...
	@echo "✅ Code looks good!"

# ✅ Full Dev Check (uses dev-check.sh)
check:
	@echo "🔍 Running comprehensive dev check..."
	@bash $(SCRIPT_DIR)/dev-check.sh

# 🗑️ Clean artifacts
clean:
	@echo "🗑️ Cleaning up..."
	$(GO) clean
	rm -f $(BINARY_NAME)
	rm -f test_output.log
	@echo "✅ Cleaned."

# ℹ️ Help
help:
	@echo "🔨 Toolsmith - Available Commands:"
	@echo "  make build    - Build the binary"
	@echo "  make run      - Run the bot"
	@echo "  make test     - Run unit tests"
	@echo "  make lint     - Run formatting and vet"
	@echo "  make check    - Run comprehensive dev check (recommended before PR)"
	@echo "  make clean    - Remove build artifacts"
	@echo "  make all      - Run check and build"
