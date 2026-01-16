.PHONY: ci fmt fmt-check lint test build run-cli run-server run-web

# Optional: pass CLI args, e.g. `make run-cli ARGS="login"`
ARGS ?=

GO_MODULES := cli server
GOLANGCI_LINT_BIN ?= $(shell command -v golangci-lint 2>/dev/null)
GOPATH_BIN := $(shell go env GOPATH)/bin

ci: fmt-check lint test build

fmt:
	@for m in $(GO_MODULES); do \
		echo "==> gofmt ($$m)"; \
		( cd $$m && gofmt -w $$(git ls-files '*.go') ); \
	done

fmt-check:
	@for m in $(GO_MODULES); do \
		echo "==> gofmt check ($$m)"; \
		out=$$(cd $$m && gofmt -l $$(git ls-files '*.go')); \
		if [ -n "$$out" ]; then \
			echo "$$out"; \
			echo "gofmt needed in $$m (run: make fmt)"; \
			exit 1; \
		fi; \
	done

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found; installing..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	}
	@LINT_BIN="$(GOLANGCI_LINT_BIN)"; \
	if [ -z "$$LINT_BIN" ] && [ -x "$(GOPATH_BIN)/golangci-lint" ]; then \
		LINT_BIN="$(GOPATH_BIN)/golangci-lint"; \
	fi; \
	if [ -z "$$LINT_BIN" ]; then \
		echo "golangci-lint installation failed (not found in PATH or $(GOPATH_BIN))"; \
		exit 1; \
	fi; \
	for m in $(GO_MODULES); do \
		echo "==> golangci-lint ($$m)"; \
		( cd $$m && $$LINT_BIN run ./... ); \
	done

test:
	@for m in $(GO_MODULES); do \
		echo "==> go test ($$m)"; \
		( cd $$m && go test ./... ); \
	done

build:
	@echo "==> go build (cli)"
	@cd cli && go build ./cmd/sentra
	@echo "==> go build (server)"
	@cd server && go build ./...
	@if [ -f web/package.json ]; then \
		echo "==> web build/lint/test"; \
		cd web && npm ci && npm run lint --if-present && npm test --if-present && npm run build --if-present; \
	else \
		echo "==> web: no package.json yet; skipping"; \
	fi

sentra:
	@cd cli && go run ./cmd/sentra $(ARGS)

run-server:
	@cd server && go run .

run-web:
	@if [ -f web/package.json ]; then \
		cd web && npm run dev --if-present; \
	else \
		echo "web/ has no package.json yet; nothing to run"; \
	fi
