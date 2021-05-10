.DEFAULT_GOAL = build

# Absolutely awesome: http://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
.PHONY: help

setup: ## Install tool dependencies
	@shed install
.PHONY: setup

clean: ## Clean all build artifacts
	@rm -rf coverage
.PHONY: clean

fmt: ## Format all go files
	@shed run goimports -- -w .
.PHONY: fmt

check-fmt: ## Check if any go files need to be formatted
	@./scripts/check_fmt.sh
.PHONY: check-fmt

lint: ## Lint go files
	@shed run golangci-lint run ./...
.PHONY: lint

# Run tests and collect coverage data
test: ## Run all tests
	@mkdir -p coverage
	@go test -coverpkg=./... -coverprofile=coverage/coverage.txt ./...
.PHONY: test

cover: test ## Run all tests and generate coverage data
	@go tool cover -html=coverage/coverage.txt -o coverage/coverage.html
.PHONY: cover
