GOFMT ?= gofmt
GOLANGCI_LINT ?= golangci-lint
GO ?= go
BIN ?= bin/netCatalog

.DEFAULT_GOAL := check
.PHONY: fmt fmt-check lint check build run test test-race vet clean

fmt:
	$(GOFMT) -w main.go app internal test

fmt-check:
	@files="$$($(GOFMT) -l main.go app internal test)" || exit $$?; \
	if [ -n "$$files" ]; then \
		printf '%s\n' "$$files"; \
		printf '%s\n' "Formatting differs; run 'make fmt'." >&2; \
		exit 1; \
	fi

lint:
	$(GOLANGCI_LINT) run --config .golangci.yml ./...

check: fmt-check
	$(MAKE) lint

build:
	$(GO) build -o $(BIN) .

# Пример: make run ARGS="get discord --type domains"
run:
	$(GO) run . $(ARGS)

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

clean:
	rm -rf bin
