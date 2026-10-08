GOFMT ?= gofmt
GOLANGCI_LINT ?= golangci-lint
GOLANGCI_LINT_VERSION := 2.14.0

.DEFAULT_GOAL := check
.PHONY: fmt fmt-check lint check

fmt:
	$(GOFMT) -w cmd internal

fmt-check:
	@files="$$($(GOFMT) -l cmd internal)" || exit $$?; \
	if [ -n "$$files" ]; then \
		printf '%s\n' "$$files"; \
		printf '%s\n' "Formatting differs; run 'make fmt'." >&2; \
		exit 1; \
	fi

lint:
	@version="$$($(GOLANGCI_LINT) version --short)" || exit $$?; \
	if [ "$$version" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		printf 'Expected golangci-lint %s, got %s. See README.md for installation.\n' \
			"$(GOLANGCI_LINT_VERSION)" "$$version" >&2; \
		exit 1; \
	fi
	$(GOLANGCI_LINT) run --config .golangci.yml ./...

check: fmt-check
	$(MAKE) lint
