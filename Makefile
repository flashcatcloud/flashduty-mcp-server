# Build configuration
BINARY_NAME := flashduty-mcp-server
BUILD_DIR := bin
GOLANGCI_LINT_VERSION := v2.2.1
GOLANGCI_LINT := $(BUILD_DIR)/golangci-lint
GCI_VERSION := v0.13.5
GCI := $(BUILD_DIR)/gci

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOFMT := gofmt
MODULE := $(shell go list -m)

# Default target
.PHONY: all
all: check

# ============================================================================
# Development targets
# ============================================================================

.PHONY: build
build: ## Build the binary
	$(GOBUILD) -v -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/$(BINARY_NAME)

.PHONY: run
run: build ## Build and run the server
	./$(BUILD_DIR)/$(BINARY_NAME)

# ============================================================================
# Quality assurance targets
# ============================================================================

# Directories to format (excludes openspec which contains reference code)
FMT_DIRS := cmd pkg internal e2e

.PHONY: fmt
fmt: $(GCI) ## Format Go source code and sort imports
	$(GOFMT) -s -w $(FMT_DIRS)
	$(GCI) write --skip-generated -s standard -s default -s "prefix($(MODULE))" $(FMT_DIRS)

.PHONY: gci
gci: $(GCI) ## Sort imports using gci
	$(GCI) write --skip-generated -s standard -s default -s "prefix($(MODULE))" $(FMT_DIRS)

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT) ## Run golangci-lint with auto-fix
	$(GOLANGCI_LINT) run --fix

.PHONY: test
test: ## Run unit tests
	$(GOTEST) -race ./...

.PHONY: test-v
test-v: ## Run unit tests with verbose output
	$(GOTEST) -race -v ./...

.PHONY: e2e
e2e: ## Run E2E tests (requires FLASHDUTY_E2E_APP_KEY and FLASHDUTY_E2E_BASE_URL)
	@if [ -z "$$FLASHDUTY_E2E_APP_KEY" ]; then \
		echo "Error: FLASHDUTY_E2E_APP_KEY environment variable is not set"; \
		exit 1; \
	fi
	$(GOTEST) -v --tags e2e ./e2e/... -timeout 10m

.PHONY: e2e-debug
e2e-debug: ## Run E2E tests in debug mode (in-process, no Docker)
	@if [ -z "$$FLASHDUTY_E2E_APP_KEY" ]; then \
		echo "Error: FLASHDUTY_E2E_APP_KEY environment variable is not set"; \
		exit 1; \
	fi
	FLASHDUTY_E2E_DEBUG=true $(GOTEST) -v --tags e2e ./e2e/... -timeout 10m

# ============================================================================
# Pre-push check (recommended before pushing)
# ============================================================================

.PHONY: check
check: fmt lint test build ## Run all checks (fmt, lint, test, build) - recommended before pushing

.PHONY: ci
ci: check ## Alias for check

# ============================================================================
# Docker targets
# ============================================================================

.PHONY: docker-build
docker-build: ## Build Docker image
	docker build -t flashcat/flashduty-mcp-server .

.PHONY: docker-e2e
docker-e2e: docker-build ## Build Docker image and run E2E tests
	@if [ -z "$$FLASHDUTY_E2E_APP_KEY" ]; then \
		echo "Error: FLASHDUTY_E2E_APP_KEY environment variable is not set"; \
		exit 1; \
	fi
	$(GOTEST) -v --tags e2e ./e2e/... -timeout 10m

# ============================================================================
# Dependency management
# ============================================================================

.PHONY: deps
deps: ## Download Go dependencies
	$(GOCMD) mod download

.PHONY: deps-tidy
deps-tidy: ## Tidy Go modules
	$(GOCMD) mod tidy

.PHONY: deps-verify
deps-verify: ## Verify Go dependencies
	$(GOCMD) mod verify

# ============================================================================
# Tools installation
# ============================================================================

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

$(GOLANGCI_LINT): $(BUILD_DIR)
	@if [ ! -f "$(GOLANGCI_LINT)" ]; then \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s $(GOLANGCI_LINT_VERSION); \
	fi

$(GCI): $(BUILD_DIR)
	@if [ ! -f "$(GCI)" ]; then \
		echo "Installing gci $(GCI_VERSION)..."; \
		GOBIN=$(CURDIR)/$(BUILD_DIR) $(GOCMD) install github.com/daixiang0/gci@$(GCI_VERSION); \
	fi

.PHONY: tools
tools: $(GOLANGCI_LINT) $(GCI) ## Install required tools

# ============================================================================
# Cleanup
# ============================================================================

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

# ============================================================================
# Help
# ============================================================================

.PHONY: help
help: ## Display this help message
	@echo "Available targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Quick start:"
	@echo "  make check    - Run all pre-push checks (recommended before pushing)"
	@echo "  make lint     - Run linter only"
	@echo "  make test     - Run unit tests only"
	@echo "  make e2e      - Run E2E tests (requires env vars)"
