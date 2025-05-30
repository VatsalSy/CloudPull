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

.PHONY: test-build
test-build: ## Test if the project builds without creating artifacts
	@echo "Testing build..."
	@$(GOBUILD) -o /dev/null ./$(MAIN_PATH)
	@echo "Build test successful"

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
		exit 1; \
	fi

.PHONY: lint-fix
lint-fix: ## Run linters with auto-fix
	@echo "Running linters with auto-fix..."
	@if command -v golangci-lint &> /dev/null; then \
		golangci-lint run --fix; \
	else \
		echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

.PHONY: lint-install
lint-install: ## Install linting tools
	@echo "Installing linting tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	@echo "Linting tools installed"

.PHONY: security
security: ## Run security checks
	@echo "Running security checks..."
	@if command -v gosec &> /dev/null; then \
		gosec -fmt json -out security-report.json ./...; \
		@echo "Security report generated: security-report.json"; \
	else \
		echo "gosec not installed. Run: go install github.com/securego/gosec/v2/cmd/gosec@latest"; \
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
	@rm -f coverage.out coverage.html security-report.json
	@echo "Clean complete"


.PHONY: update-deps
update-deps: ## Update dependencies
	@echo "Updating dependencies..."
	$(GOMOD) get -u ./...
	$(GOMOD) tidy

.PHONY: verify
verify: fmt vet lint test ## Run fmt, vet, lint, and tests
	@echo "Verification complete"

.PHONY: quick-check
quick-check: fmt vet test-build ## Quick checks: format, vet, and build test
	@echo "Quick check complete"

.PHONY: pre-commit
pre-commit: ## Run pre-commit checks on all files
	@echo "Running pre-commit checks..."
	@if command -v pre-commit &> /dev/null; then \
		pre-commit run --all-files; \
	else \
		echo "pre-commit not installed. Run: ./scripts/install-hooks.sh"; \
	fi

.PHONY: pre-commit-install
pre-commit-install: ## Install pre-commit hooks
	@echo "Installing pre-commit hooks..."
	@./scripts/install-hooks.sh

.PHONY: pre-commit-update
pre-commit-update: ## Update pre-commit hooks
	@echo "Updating pre-commit hooks..."
	@if command -v pre-commit &> /dev/null; then \
		pre-commit autoupdate; \
	else \
		echo "pre-commit not installed. Run: ./scripts/install-hooks.sh"; \
	fi

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