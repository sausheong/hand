# ──────────────────────────────────────────────────────────────
# Hand — Makefile
# ──────────────────────────────────────────────────────────────

.PHONY: help build run test vet fmt lint install deploy clean dist release

BIN_DIR := bin
DIST_DIR := dist
VERSION ?= $(shell git describe --tags --always --dirty)
PLATFORMS := darwin-amd64 darwin-arm64 linux-amd64 linux-arm64

# ── Default ────────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

# ── Build ──────────────────────────────────────────────────────

build: ## Build the hand binary to bin/hand
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/hand ./cmd/hand

run: ## Run hand without building (fast dev loop)
	go run ./cmd/hand

install: ## Install hand to $GOPATH/bin (or $GOBIN)
	go install ./cmd/hand

deploy: install ## Alias for install

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

# ── Release ────────────────────────────────────────────────────

dist: ## Cross-compile release archives for all platforms into dist/ (set VERSION=vX.Y.Z; defaults to `git describe`)
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)
	@for plat in $(PLATFORMS); do \
		os=$$(echo $$plat | cut -d- -f1); \
		arch=$$(echo $$plat | cut -d- -f2); \
		name=hand-$(VERSION)-$$plat; \
		echo "building $$name..."; \
		mkdir -p $(DIST_DIR)/$$name; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -o $(DIST_DIR)/$$name/hand ./cmd/hand || exit 1; \
		cp README.md $(DIST_DIR)/$$name/; \
		tar -C $(DIST_DIR) -czf $(DIST_DIR)/$$name.tar.gz $$name; \
		rm -rf $(DIST_DIR)/$$name; \
	done
	@cd $(DIST_DIR) && (command -v sha256sum >/dev/null 2>&1 && sha256sum *.tar.gz || shasum -a 256 *.tar.gz) > SHA256SUMS

release: ## Tag and push a release (usage: make release VERSION=v0.1.0) — the GitHub Action builds dist/ and publishes it
ifneq ($(origin VERSION),command line)
	$(error usage: make release VERSION=vX.Y.Z)
endif
	git tag -a $(VERSION) -m "$(VERSION)"
	git push origin $(VERSION)

# ── Clean ──────────────────────────────────────────────────────

clean: ## Remove built artifacts
	rm -rf $(BIN_DIR) $(DIST_DIR)
