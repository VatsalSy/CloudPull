# CloudPull Makefile

# Variables
BINARY_NAME=cloudpull
BINARY_DIR=bin
CMD_DIR=cmd/cloudpull
VERSION=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date +%Y%m%d-%H%M%S)
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Targets
.PHONY: all build clean test deps run install uninstall

all: clean deps build

build:
	@echo "Building CloudPull..."
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Build complete: $(BINARY_DIR)/$(BINARY_NAME)"

clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BINARY_DIR)

test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

run: build
	@echo "Running CloudPull..."
	./$(BINARY_DIR)/$(BINARY_NAME)

install: build
	@echo "Installing CloudPull..."
	@cp $(BINARY_DIR)/$(BINARY_NAME) /usr/local/bin/
	@echo "CloudPull installed to /usr/local/bin/$(BINARY_NAME)"
	@echo "Run 'cloudpull init' to get started"

uninstall:
	@echo "Uninstalling CloudPull..."
	@rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "CloudPull uninstalled"

# Development helpers
.PHONY: fmt vet lint

fmt:
	@echo "Formatting code..."
	@gofmt -s -w .

vet:
	@echo "Running go vet..."
	@go vet ./...

lint:
	@echo "Running linter..."
	@golangci-lint run

# Build for multiple platforms
.PHONY: build-all build-linux build-windows build-darwin

build-all: build-linux build-windows build-darwin

build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BINARY_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_DIR)

build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BINARY_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BINARY_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)

# Help
help:
	@echo "CloudPull Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make              Build CloudPull"
	@echo "  make build        Build the binary"
	@echo "  make clean        Clean build artifacts"
	@echo "  make test         Run tests"
	@echo "  make deps         Download dependencies"
	@echo "  make run          Build and run CloudPull"
	@echo "  make install      Install CloudPull to /usr/local/bin"
	@echo "  make uninstall    Uninstall CloudPull"
	@echo "  make fmt          Format code"
	@echo "  make vet          Run go vet"
	@echo "  make lint         Run linter"
	@echo "  make build-all    Build for all platforms"
	@echo "  make help         Show this help message"