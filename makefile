.PHONY: ci fmt fmt-check lint test build run-cli run-server run-web

# Optional: pass CLI args, e.g. `make run-cli ARGS="login"`
ARGS ?=

GO_MODULES := cli server
GOLANGCI_LINT_BIN ?= $(shell command -v golangci-lint 2>/dev/null)
GOPATH_BIN := $(shell go env GOPATH)/bin

sentra:
	@cd cli && go run ./cmd/sentra $(ARGS)

run-server:
	@cd server && go run .

run-web:
	@if [ -f web/package.json ]; then \
		cd web && pnpm run dev --if-present; \
	else \
		echo "web/ has no package.json yet; nothing to run"; \
	fi
