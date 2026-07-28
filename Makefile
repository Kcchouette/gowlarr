.PHONY: build test lint fmt vet clean all ci coverage help

GOLANGCI_LINT_VERSION := v1.64.5

help: ## Afficher cette aide
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Compiler le binaire
	go build ./cmd/gowlarr

test: ## Lancer les tests avec race detector
	go test -race -count=1 ./...

lint: ## Lancer golangci-lint
	golangci-lint run

fmt: ## Formater le code
	gofmt -s -w .
	goimports -w .

vet: ## Lancer go vet
	go vet ./...

clean: ## Nettoyer les artefacts
	rm -f gowlarr coverage.out

coverage: ## Générer le rapport de couverture
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

all: fmt vet lint test build ## Tout exécuter (format + lint + test + build)

ci: all ## Alias pour CI
