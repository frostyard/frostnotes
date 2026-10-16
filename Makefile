.PHONY: help build fmt gen gen-check lint lint-version-check test verify check ci clean

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

# golangci-lint's release, read from mise.toml (frostyard/core ADR-0043).
GOLANGCI_LINT_VERSION := $(strip $(shell sed -n 's/^golangci-lint = "\(.*\)"/\1/p' mise.toml))
# Tracked or new Go files outside docs/tasks (given files are checked when
# copied into place), skipping files deleted but not yet staged.
GO_FILES = $(shell git ls-files --cached --others --exclude-standard '*.go' | grep -v -e '^docs/tasks/' | while read -r f; do [ -e "$$f" ] && echo "$$f"; done)

help: ## List targets
	@grep -E '^[a-z0-9-]+:.*## ' $(MAKEFILE_LIST) | sed 's/:.*## /\t/' | expand -t 18

build: ## Build notesd and notesctl into build/
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o build/ ./cmd/...

fmt: ## Format Go code
	gofmt -w $(GO_FILES)

gen: ## Regenerate the RPC contract from schema/rpc
	go run ./tools/rpcgen

gen-check: ## Fail if generated RPC files are stale
	go run ./tools/rpcgen -check

lint: lint-version-check ## Run the pinned golangci-lint
	golangci-lint run

lint-version-check:
	@test -n "$(GOLANGCI_LINT_VERSION)" || { echo "mise.toml pins no golangci-lint"; exit 1; }
	@installed="$$(golangci-lint version --short 2>/dev/null)" || { \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) is required (install with: mise install)"; exit 1; }; \
	if [ "$$installed" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "expected golangci-lint $(GOLANGCI_LINT_VERSION), found $$installed (install with: mise install)"; exit 1; \
	fi

# Unit runs skip TestI*, which frostyard/core ADR-0022 reserves for
# integration tests.
test: ## Run the Go unit tests
	go test -run '^Test[^I]' ./...

# The gate triad (frostyard/core ADR-0043, ADR-0044). verify is credential-free
# and leaves a clean checkout clean; check formats first; ci adds the race
# detector.
verify: ## Tidy, generated files, vet, format, lint and unit tests
	go mod tidy -diff
	$(MAKE) --no-print-directory gen-check
	go vet ./...

	test -z "$$(gofmt -l $(GO_FILES))"
	$(MAKE) --no-print-directory lint
	$(MAKE) --no-print-directory test

check: fmt verify ## Format, then verify (the developer gate)

ci: verify ## verify plus race tests
	go test -race -run '^Test[^I]' ./...

clean: ## Remove build output
	rm -rf build dist coverage.out coverage.html

# Task cards (docs/design/agent-workflow.md). The executor is Codex with the
# user's model unless EXECUTOR_MODEL names one; or opencode with
# EXECUTOR_MODEL as its provider/model (ADR-0010).
.PHONY: tasks accept task task-verify task-finish
EXECUTOR ?= codex
EXECUTOR_MODEL ?=

tasks: ## List task cards
	@go run ./tools/taskrun list

accept: ## Run a task card's acceptance command: make accept T=0001
	@test -n "$(T)" || { echo "usage: make accept T=NNNN"; exit 2; }
	go run ./tools/taskrun accept $(T)

task: ## Run a task card through the executor (Codex by default): make task T=0001
	@test -n "$(T)" || { echo "usage: make task T=NNNN"; exit 2; }
	go run ./tools/taskrun -executor '$(EXECUTOR)' -model '$(EXECUTOR_MODEL)' run $(T)

task-verify: ## Check a task branch's scope, given files and gates
	go run ./tools/taskrun verify $(T)

task-finish: ## Verify, move the card to done/ and commit the task's work
	go run ./tools/taskrun finish $(T)
