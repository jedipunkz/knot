BINARY  := knot
PKG     := ./cmd/knot
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
STATICCHECK_VERSION := v0.8.2

.PHONY: all build install run test test-race vet fmt fmt-check lint check clean help

all: check build

build: ## Build ./knot
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) $(PKG)

install: ## Install knot into $GOBIN (or $GOPATH/bin)
	go install -trimpath -ldflags '$(LDFLAGS)' $(PKG)

run: build ## Build and run knot (pass arguments with ARGS="...")
	./$(BINARY) $(ARGS)

test: ## Run tests
	go test ./...

test-race: ## Run tests with the race detector
	go test -race ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format the code
	gofmt -w .

fmt-check: ## Fail if the code is not formatted
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "$$out"; exit 1; fi

lint: ## Run staticcheck
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

check: fmt-check vet lint test-race ## Run all checks (same as CI)

clean: ## Remove the built binary
	rm -f $(BINARY)

help: ## Show targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'
