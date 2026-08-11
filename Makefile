.PHONY: init test-coverage test-coverage-html lint fmt help

init: ## Install utility tools for development lifecycle
	go install github.com/evilmartians/lefthook@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	lefthook install

test-coverage: ## Run all tests and compute coverage
	go test -v -coverprofile=coverage.out \
		./internal/store \
		./internal/service \
		./internal/infra/migrations \
		./internal/infra/query \
		./internal/api/handler \
		./internal/api/utils

	@go tool cover -func=coverage.out

test-coverage-html: test-coverage ## Generate HTML coverage report
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

lint: ## Run linter
	golangci-lint run ./...

fmt: ## Format and organize imports
	goimports -w .
	go fmt ./...

help: ## Show help for each make command
	@echo 'Makefile commands:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'