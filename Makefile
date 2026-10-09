GOFMT ?= gofmt
GOLANGCI_LINT ?= golangci-lint

.DEFAULT_GOAL := check
.PHONY: fmt fmt-check lint check

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

run: go run .
