.PHONY: help build run clean test lint tidy deps ui-deps ui-build ui-dev build-all clean-all

# Binary name
BINARY_NAME=dc-switcher
MAIN_PATH=./cmd/dc-switcher
UI_PATH=./ui

# Build variables
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@go mod verify

tidy: ## Tidy go modules
	@echo "Tidying go modules..."
	@go mod tidy

build: deps ## Build the application
	@echo "Building $(BINARY_NAME)..."
	@go build $(LDFLAGS) -o bin/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: bin/$(BINARY_NAME)"

run: ## Run the application
	@echo "Running $(BINARY_NAME)..."
	@go run $(MAIN_PATH) -config config.yaml

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf bin/
	@go clean

clean-all: clean ## Clean all build artifacts including UI
	@echo "Cleaning UI..."
	@rm -rf $(UI_PATH)/dist/ $(UI_PATH)/node_modules/

test: ## Run tests
	@echo "Running tests..."
	@go test -v -race -cover ./...

lint: ## Run linter
	@echo "Running linter..."
	@which golangci-lint > /dev/null 2>&1 || (echo "golangci-lint not installed. Install from https://golangci-lint.run/usage/install/" && exit 1)
	@golangci-lint run ./...

fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

check: fmt vet lint test ## Run all checks (fmt, vet, lint, test)

install: build ## Install the binary
	@echo "Installing $(BINARY_NAME)..."
	@cp bin/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)
	@echo "Installed to $(GOPATH)/bin/$(BINARY_NAME)"

# UI commands
ui-deps: ## Install UI dependencies
	@echo "Installing UI dependencies..."
	@cd $(UI_PATH) && npm install

ui-build: ui-deps ## Build UI
	@echo "Building UI..."
	@cd $(UI_PATH) && npm run build
	@echo "UI build complete: $(UI_PATH)/dist"

ui-dev: ui-deps ## Run UI in development mode
	@echo "Starting UI development server..."
	@cd $(UI_PATH) && npm run dev

build-all: ui-build build ## Build both UI and backend

.DEFAULT_GOAL := help
