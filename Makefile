.PHONY: build test lint fmt vet clean all ci coverage help

help: ## Display this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	go build ./cmd/gowlarr

test: ## Run tests with the race detector
	go test -race -count=1 ./...

lint: ## Run golangci-lint
	golangci-lint run

fmt: ## Format the code
	gofmt -s -w .
	goimports -w .

vet: ## Run go vet
	go vet ./...

clean: ## Remove build artifacts
	rm -f gowlarr coverage.out

coverage: ## Generate the coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

all: fmt vet lint test build ## Run everything (format + lint + test + build)

ci: all ## CI alias
