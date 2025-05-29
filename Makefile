# CloudPull Makefile

# Variables
BINARY_NAME=cloudpull
BINARY_DIR=bin
GO_CMD=go
GO_BUILD=$(GO_CMD) build
GO_TEST=$(GO_CMD) test
GO_CLEAN=$(GO_CMD) clean
GO_GET=$(GO_CMD) get
GO_MOD=$(GO_CMD) mod
MAIN_PATH=./cmd/cloudpull

# Build variables
VERSION?=dev
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT_HASH=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.CommitHash=$(COMMIT_HASH)"

# Platforms
PLATFORMS=darwin linux windows
ARCHITECTURES=amd64 arm64

.PHONY: all build clean test coverage deps run install lint fmt help

# Default target
all: clean deps build test

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BINARY_DIR)
	$(GO_BUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME) $(MAIN_PATH)

# Build for all platforms
build-all:
	@echo "Building for all platforms..."
	@mkdir -p $(BINARY_DIR)
	@for platform in $(PLATFORMS); do \
		for arch in $(ARCHITECTURES); do \
			output_name=$(BINARY_NAME)-$$platform-$$arch; \
			if [ $$platform = "windows" ]; then output_name="$$output_name.exe"; fi; \
			echo "Building $$output_name..."; \
			GOOS=$$platform GOARCH=$$arch $(GO_BUILD) $(LDFLAGS) -o $(BINARY_DIR)/$$output_name $(MAIN_PATH); \
		done; \
	done

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@$(GO_CLEAN)
	@rm -rf $(BINARY_DIR)

# Run tests
test:
	@echo "Running tests..."
	$(GO_TEST) -v -race ./...

# Run tests with coverage
coverage:
	@echo "Running tests with coverage..."
	$(GO_TEST) -v -race -coverprofile=coverage.out ./...
	$(GO_CMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GO_MOD) download
	$(GO_MOD) tidy

# Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_DIR)/$(BINARY_NAME)

# Install the binary
install: build
	@echo "Installing $(BINARY_NAME)..."
	@cp $(BINARY_DIR)/$(BINARY_NAME) $(GOPATH)/bin/

# Lint the code
lint:
	@echo "Linting..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found, installing..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run

# Format the code
fmt:
	@echo "Formatting code..."
	$(GO_CMD) fmt ./...

# Generate mocks
mocks:
	@echo "Generating mocks..."
	@which mockgen > /dev/null || (echo "mockgen not found, installing..." && go install github.com/golang/mock/mockgen@latest)
	go generate ./...

# Database migrations
migrate-up:
	@echo "Running database migrations..."
	@# Add migration command here

migrate-down:
	@echo "Rolling back database migrations..."
	@# Add rollback command here

# Development setup
dev-setup:
	@echo "Setting up development environment..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/golang/mock/mockgen@latest
	@go install github.com/goreleaser/goreleaser@latest
	@echo "Development tools installed!"

# Help
help:
	@echo "CloudPull Makefile commands:"
	@echo "  make build       - Build the binary"
	@echo "  make build-all   - Build for all platforms"
	@echo "  make clean       - Clean build artifacts"
	@echo "  make test        - Run tests"
	@echo "  make coverage    - Run tests with coverage"
	@echo "  make deps        - Download dependencies"
	@echo "  make run         - Build and run the application"
	@echo "  make install     - Install the binary"
	@echo "  make lint        - Lint the code"
	@echo "  make fmt         - Format the code"
	@echo "  make dev-setup   - Install development tools"
	@echo "  make help        - Show this help message"