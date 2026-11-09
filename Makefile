.PHONY: init proto tests coverage test-report e2e vm-up vm-test vm-down vm-clean build help

# TEST_PACKAGES declares which packages should be executed for tests, keeping
# untested packages out of the coverage profile.
TEST_PACKAGES = ./internal/orchestrator/store \
	./internal/common/rules \
	./internal/common/propagation \
	./internal/orchestrator/api/grpcserver \
	./internal/sentinel/propagation \
	./internal/orchestrator/service \
	./internal/orchestrator/metrics \
	./internal/orchestrator/config \
	./internal/common/auth \
	./internal/orchestrator/api \
	./internal/sentinel/config \
	./internal/sentinel/heartbeat \
	./internal/sentinel/identity \
	./internal/sentinel/client \
	./internal/sentinel/filtering \
	./internal/sentinel/hostresolve \
	./internal/sentinel/filtering/nftables \
	./internal/sentinel/filtering/dnsserver \
	./internal/orchestrator/infra/migrations \
	./internal/orchestrator/infra/query \
	./internal/orchestrator/api/handler \
	./internal/orchestrator/api/utils

init: ## Install utility tools for development lifecycle
	go install github.com/evilmartians/lefthook@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/dougdomingos/test-prettify@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	lefthook install

proto: ## Generate gRPC code from the propagation proto contract
	@mkdir -p pkg/pb
	protoc --proto_path=proto \
		--go_out=pkg/pb --go_opt=paths=source_relative \
		--go-grpc_out=pkg/pb --go-grpc_opt=paths=source_relative \
		proto/propagation.proto

build: proto ## Build both binaries into ./bin
	@mkdir -p bin
	@go build -o bin/aegis-orchestrator ./cmd/orchestrator
	@go build -o bin/aegis-sentinel ./cmd/sentinel

tests: proto ## Run all tests and report errors only
	@go test -short $(TEST_PACKAGES)

coverage: proto ## Run all tests and compute coverage
	@go test -v -coverprofile=coverage.out $(TEST_PACKAGES)
	@go tool cover -func=coverage.out

test-report: proto ## Run all tests and yield report as HTML
	@go test -v -json -coverprofile=coverage.out $(TEST_PACKAGES) | test-prettify --cov-prof coverage.out

e2e: proto ## Run the end-to-end suite with Testcontainers (requires Docker)
	@go test -tags e2e -timeout 30m -v ./e2e/containers/...

vm-up: ## Provision the KVM lab (first run downloads and builds the base image)
	@e2e/vm/scripts/up.sh

vm-test: proto ## Run the filtering suite against the KVM lab
	@e2e/vm/scripts/check-build.sh
	@go test -tags vm -count=1 -p 1 -timeout 30m -v ./e2e/vm/...

vm-down: ## Remove the KVM lab, keeping the cached base image
	@e2e/vm/scripts/down.sh

vm-clean: ## Remove the KVM lab, the base image and the storage pool
	@e2e/vm/scripts/clean.sh

help: ## Show help for each make command
	@echo 'Makefile commands:'
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'