# ──────────────────────────────────────────────────────────────
# agcode — Makefile
# ──────────────────────────────────────────────────────────────

.PHONY: help build run test vet fmt lint install clean

BIN_DIR := bin

# ── Default ────────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

# ── Build ──────────────────────────────────────────────────────

build: ## Build the agcode binary to bin/agcode
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/agcode ./cmd/agcode

run: ## Run agcode without building (fast dev loop)
	go run ./cmd/agcode

install: ## Install agcode to $GOPATH/bin (or $GOBIN)
	go install ./cmd/agcode

# ── Go tools ───────────────────────────────────────────────────

test: ## Run go test ./...
	go test ./...

vet: ## Run go vet ./...
	go vet ./...

fmt: ## Run go fmt ./...
	go fmt ./...

lint: ## Run golangci-lint (if installed)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed — run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# ── Clean ──────────────────────────────────────────────────────

clean: ## Remove built artifacts
	rm -rf $(BIN_DIR)
