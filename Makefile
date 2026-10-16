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

ci: verify ui-check app-build app-test ## verify plus race tests and the app checks (nsl)
	go test -race -run '^Test[^I]' ./...

clean: ## Remove build output
	rm -rf build dist coverage.out coverage.html

# The app (M0 Phase 4). Its toolchain runs in the nsl machine
# (docs/adr/0009-development-environment-and-gates.md); CI, whose runner has
# the toolchain, runs the same targets with IN_NSL='sh -c'.
.PHONY: ui-check ui-test ui-vitest ui-fmt ui-dev app-build app-test app-dev app-run spike-cm6 spike-cm6-nsl

NSL_MACHINE ?= frostnotes
IN_NSL := nsl run -m $(NSL_MACHINE) sh -lc
# WebKitGTK without a display: Xvfb, so software rendering.
XVFB := env -u WAYLAND_DISPLAY GDK_BACKEND=x11 xvfb-run -a -s "-screen 0 1280x800x24"

ui-check: ## Lint (Biome), typecheck and test the app UI (nsl)
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm run check'

ui-test: ui-check ## Alias for ui-check

ui-vitest: ## Run app tests matching F (a path or pattern) in the nsl machine
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm exec vitest run $(F)'

ui-fmt: ## Format the app's sources and organize imports with Biome (nsl)
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm run fmt'

ui-dev: build ## Run notesd -devgw and Vite in the nsl machine; open http://127.0.0.1:5173 on the host
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && cd .. && scripts/dev-app.sh --browser'

app-build: ## Build the release app binary into build/frostnotes-app (nsl)
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm tauri build && mkdir -p ../build && cp "$$CARGO_TARGET_DIR/release/frostnotes" ../build/frostnotes-app'

app-test: ## Run the app shell's Rust tests (nsl; needs app/dist from app-build)
	$(IN_NSL) 'cd app/src-tauri && cargo test --release'

app-dev: build ## Run notesd and tauri dev in the nsl machine
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && cd .. && scripts/dev-app.sh'

app-run: build app-build ## Run notesd and the built app in the nsl machine
	$(IN_NSL) 'APP_BINARY=build/frostnotes-app scripts/dev-app.sh'

# M0 spike 1 (docs/plans/0001-m0-foundations.md, Phase 5): the release app
# runs the CodeMirror spike, prints its results as one JSON line and exits.
# SPIKE=cm6:manual only opens the spike's editor, for the IME check by hand.
SPIKE ?= cm6
spike-cm6: ## Run spike 1 natively on the host GPU with build/frostnotes-app (from make app-build)
	@test -x build/frostnotes-app || { echo "build/frostnotes-app is missing; run make app-build first"; exit 1; }
	FROSTNOTES_SPIKE=$(SPIKE) build/frostnotes-app

spike-cm6-nsl: ## Run spike 1 in the nsl machine under Xvfb (software rendering)
	@test -x build/frostnotes-app || { echo "build/frostnotes-app is missing; run make app-build first"; exit 1; }
	$(IN_NSL) 'FROSTNOTES_SPIKE=cm6 $(XVFB) build/frostnotes-app'
