.PHONY: init tests coverage test-report help

# TEST_PACKAGES declares which packages should be executed for tests, keeping
# untested packages out of the coverage profile.
TEST_PACKAGES = ./internal/store \
	./internal/service \
	./internal/infra/migrations \
	./internal/infra/query \
	./internal/api/handler \
	./internal/api/utils

init: ## Install utility tools for development lifecycle
	go install github.com/evilmartians/lefthook@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/dougdomingos/test-prettify@latest
	go install golang.org/x/tools/cmd/goimports@latest
	lefthook install

tests: ## Run all tests and report errors only
	@go test -short $(TEST_PACKAGES)

coverage: ## Run all tests and compute coverage
	@go test -v -coverprofile=coverage.out $(TEST_PACKAGES)
	@go tool cover -func=coverage.out

test-report: ## Run all tests and yield report as HTML
	@go test -v -json -coverprofile=coverage.out $(TEST_PACKAGES) | test-prettify --cov-prof coverage.out

help: ## Show help for each make command
	@echo 'Makefile commands:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'