.PHONY: ci cli-ci server-ci web-ci fmt-check-cli fmt-check-server lint-cli lint-server test-cli test-server build-cli build-server fmt fmt-check lint test build run-cli run-server run-web

# CLI CI pipeline: build, format check, lint, test
cli-ci: build-cli fmt-check-cli lint-cli test-cli

build-cli:
	@echo "=== Building CLI ==="
	@cd cli && go build ./cmd/sentra

fmt-check-cli:
	@echo "=== Checking CLI format (gofmt) ==="
	@cd cli && \
	files="$$(git ls-files '*.go' 2>/dev/null || find . -name '*.go' -type f)" && \
	if [ -z "$$files" ]; then \
		echo "No Go files found"; \
		exit 0; \
	fi && \
	out="$$(gofmt -l $$files)" && \
	if [ -n "$$out" ]; then \
		echo "Files need formatting:"; \
		echo "$$out"; \
		echo "Run: gofmt -w $$files"; \
		exit 1; \
	fi && \
	echo "✓ All files formatted correctly"

lint-cli:
	@echo "=== Linting CLI ==="
	@GOPATH_BIN=$$(go env GOPATH)/bin; \
	if [ -z "$(GOLANGCI_LINT_BIN)" ]; then \
		echo "golangci-lint not found. Installing..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$GOPATH_BIN latest; \
	fi; \
	GOLANGCI_LINT="$$GOPATH_BIN/golangci-lint"; \
	if [ -n "$(GOLANGCI_LINT_BIN)" ]; then \
		GOLANGCI_LINT="$(GOLANGCI_LINT_BIN)"; \
	fi; \
	cd cli && $$GOLANGCI_LINT run ./...

test-cli:
	@echo "=== Testing CLI ==="
	@cd cli && go test ./...

# Server CI pipeline: build, format check, lint, test
server-ci: build-server fmt-check-server lint-server test-server

build-server:
	@echo "=== Building Server ==="
	@cd server && go build ./...

fmt-check-server:
	@echo "=== Checking Server format (gofmt) ==="
	@cd server && \
	files="$$(git ls-files '*.go' 2>/dev/null || find . -name '*.go' -type f)" && \
	if [ -z "$$files" ]; then \
		echo "No Go files found"; \
		exit 0; \
	fi && \
	out="$$(gofmt -l $$files)" && \
	if [ -n "$$out" ]; then \
		echo "Files need formatting:"; \
		echo "$$out"; \
		echo "Run: gofmt -w $$files"; \
		exit 1; \
	fi && \
	echo "✓ All files formatted correctly"

lint-server:
	@echo "=== Linting Server ==="
	@GOPATH_BIN=$$(go env GOPATH)/bin; \
	if [ -z "$(GOLANGCI_LINT_BIN)" ]; then \
		echo "golangci-lint not found. Installing..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$GOPATH_BIN latest; \
	fi; \
	GOLANGCI_LINT="$$GOPATH_BIN/golangci-lint"; \
	if [ -n "$(GOLANGCI_LINT_BIN)" ]; then \
		GOLANGCI_LINT="$(GOLANGCI_LINT_BIN)"; \
	fi; \
	cd server && $$GOLANGCI_LINT run ./...

test-server:
	@echo "=== Testing Server ==="
	@cd server && go test ./...

# Web CI pipeline: install, lint, test, build
web-ci:
	@echo "=== Web CI Pipeline ==="
	@if [ ! -f web/package.json ]; then \
		echo "web/ has no package.json yet; skipping web CI."; \
		exit 0; \
	fi; \
	cd web && npm ci && \
	npm run lint --if-present && \
	npm test --if-present && \
	npm run build --if-present && \
	echo "✓ Web CI completed"

# Convenience: run all CI pipelines
ci: cli-ci server-ci web-ci
