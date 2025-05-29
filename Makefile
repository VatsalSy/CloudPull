# CloudPull Makefile
# Build, test, and manage the CloudPull application

# Variables
BINARY_NAME=cloudpull
MAIN_PATH=cmd/cloudpull
BUILD_DIR=build
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"

# Go commands
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOFMT=$(GOCMD) fmt
GOMOD=$(GOCMD) mod
GORUN=$(GOCMD) run

# Default target
.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: deps
deps: ## Download and verify dependencies
	@echo "Downloading dependencies..."
	@$(GOMOD) download
	@$(GOMOD) verify
	@echo "Dependencies downloaded and verified"

.PHONY: build
build: deps ## Build the CloudPull binary
	@echo "Building CloudPull..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

.PHONY: install
install: build ## Install CloudPull to GOPATH/bin
	@echo "Installing CloudPull..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "CloudPull installed to $(GOPATH)/bin/$(BINARY_NAME)"

.PHONY: run
run: ## Run CloudPull directly
	$(GORUN) ./$(MAIN_PATH) $(ARGS)

.PHONY: test
test: ## Run all tests
	@echo "Running tests..."
	$(GOTEST) -v -race -cover ./...

.PHONY: test-unit
test-unit: ## Run unit tests only
	@echo "Running unit tests..."
	$(GOTEST) -v -race -short ./...

.PHONY: test-integration
test-integration: ## Run integration tests
	@echo "Running integration tests..."
	$(GOTEST) -v -race -run Integration ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: benchmark
benchmark: ## Run benchmarks
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

.PHONY: lint
lint: ## Run linters
	@echo "Running linters..."
	@if command -v golangci-lint &> /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting code..."
	$(GOFMT) ./...

.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	$(GOVET) ./...

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "Clean complete"

.PHONY: deps
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

.PHONY: update-deps
update-deps: ## Update dependencies
	@echo "Updating dependencies..."
	$(GOMOD) get -u ./...
	$(GOMOD) tidy

.PHONY: verify
verify: fmt vet test ## Run fmt, vet, and tests
	@echo "Verification complete"

# Development helpers

.PHONY: dev-init
dev-init: ## Initialize development environment
	@echo "Initializing development environment..."
	@mkdir -p ~/.cloudpull
	@echo "Creating example config..."
	@cp -n examples/config.yaml ~/.cloudpull/config.yaml || true
	@echo "Development environment ready"

.PHONY: dev-auth
dev-auth: ## Run authentication flow
	$(GORUN) ./$(MAIN_PATH) auth

.PHONY: dev-sync
dev-sync: ## Run example sync
	$(GORUN) ./$(MAIN_PATH) sync --output ~/CloudPull/DevTest

.PHONY: example
example: ## Run full sync example
	$(GORUN) ./examples/full_sync_example.go $(ARGS)

# Docker targets

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t cloudpull:$(VERSION) .

.PHONY: docker-run
docker-run: ## Run CloudPull in Docker
	docker run -it --rm \
		-v ~/.cloudpull:/root/.cloudpull \
		-v ~/CloudPull:/data \
		cloudpull:$(VERSION) $(ARGS)

# Release targets

.PHONY: release-dry
release-dry: ## Dry run of release process
	@echo "Dry run of release process..."
	goreleaser release --snapshot --skip-publish --rm-dist

.PHONY: release
release: ## Create a new release
	@echo "Creating release..."
	@if [ -z "$(VERSION)" ]; then \
		echo "VERSION is not set. Use: make release VERSION=v1.0.0"; \
		exit 1; \
	fi
	git tag -a $(VERSION) -m "Release $(VERSION)"
	git push origin $(VERSION)
	goreleaser release --rm-dist

# Database management

.PHONY: db-migrate
db-migrate: ## Run database migrations
	@echo "Running database migrations..."
	$(GORUN) ./$(MAIN_PATH) db migrate

.PHONY: db-reset
db-reset: ## Reset database
	@echo "Resetting database..."
	@rm -f ~/.cloudpull/cloudpull.db
	@echo "Database reset complete"

# Utility targets

.PHONY: check-tools
check-tools: ## Check required tools
	@echo "Checking required tools..."
	@command -v go >/dev/null 2>&1 || { echo "Go not installed"; exit 1; }
	@command -v git >/dev/null 2>&1 || { echo "Git not installed"; exit 1; }
	@echo "All required tools are installed"

.PHONY: info
info: ## Show build information
	@echo "CloudPull Build Information:"
	@echo "  Version: $(VERSION)"
	@echo "  Build Time: $(BUILD_TIME)"
	@echo "  Go Version: $(shell go version)"
	@echo "  Platform: $(shell go env GOOS)/$(shell go env GOARCH)"

# Shortcuts
.PHONY: b t r c
b: build ## Shortcut for build
t: test ## Shortcut for test  
r: run ## Shortcut for run
c: clean ## Shortcut for clean