.PHONY: help build fmt gen gen-check lint lint-version-check svu-version-check test verify check ci vulncheck clean deb deb-check bump

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

# golangci-lint's release, read from mise.toml (frostyard/core ADR-0043).
GOLANGCI_LINT_VERSION := $(strip $(shell sed -n 's/^golangci-lint = "\(.*\)"/\1/p' mise.toml))
# svu's release, read from the same file; make bump tags releases with it.
SVU_VERSION := $(strip $(shell sed -n 's/^svu = "\(.*\)"/\1/p' mise.toml))
SVU ?= svu
# govulncheck's release; the Go checksum database verifies the module.
GOVULNCHECK_VERSION := v1.9.0
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

# M1's check of link resolution against Obsidian's own answers for a real
# vault (docs/plans/0003-m1-headless-vault.md, Phase 2). The dump and the
# vault stay outside the repository; the vault is only walked for names.
.PHONY: obsidian-links
obsidian-links: ## Check links against Obsidian's dump (FROSTNOTES_OBSIDIAN_LINKS, FROSTNOTES_OBSIDIAN_VAULT)
	@test -n "$$FROSTNOTES_OBSIDIAN_LINKS" -a -n "$$FROSTNOTES_OBSIDIAN_VAULT" || { \
		echo "set FROSTNOTES_OBSIDIAN_LINKS to the dump and FROSTNOTES_OBSIDIAN_VAULT to the vault"; exit 2; }
	go test -tags integration -run '^TestIObsidianLinkDump$$' -count=1 -v ./internal/markdown

# The kill -9 harness (docs/plans/0003-m1-headless-vault.md, Phase 4): a
# child notesd is killed at random points of writes and scans over a
# generated vault, FROSTNOTES_KILL_RUNS times each (default 100).
.PHONY: vault-it
vault-it: ## Kill notesd during writes and scans, and check every restart
	go test -tags integration -run '^TestIKillNotesd$$' -count=1 -v -timeout 30m ./internal/vault

# Sync's seeded simulation (docs/plans/0004-m2-sync.md, Phase 5; ADR-0020):
# SIM_SEEDS seeds (default 10) of SIM_STEPS steps (default 10,000) over a hub
# and three devices, stepwise and concurrent (SIM_MODE picks one), then the
# reduced regression plans in internal/synctest/testdata. SIM_REDUCE=1
# reduces a failure and prints its plan.
.PHONY: sync-sim
sync-sim: ## Run sync's seeded simulation (SIM_SEEDS, SIM_STEPS, SIM_MODE)
	go test -tags integration -run '^(TestISimulation|TestRegressions)$$' -count=1 -v -timeout 4h ./internal/synctest

# M2's exit evidence 1 and 2 (docs/plans/0004-m2-sync.md, Phase 7): the
# seeded simulation's 10,000 steps per seed in each mode, and a 100 MB
# attachment through a hub between three notesd processes in bounded memory.
# Evidence 3, the 7-day trial on the user's vault, is the user's to run.
.PHONY: m2-evidence
m2-evidence: ## Record M2's exit evidence 1-2 (the simulation; 100 MB across three notesd)
	$(MAKE) --no-print-directory sync-sim
	go test -tags integration -run '^TestIHundredMegabyteAttachment$$' -count=1 -v -timeout 10m ./internal/syncer

# M1's exit evidence 1 to 4 (docs/plans/0003-m1-headless-vault.md, Phase 7):
# Obsidian's link dump, the user's vault indexed cold (read only), a vim save
# searchable within 1 s, and the kill -9 harness. Set TMPDIR to put the test
# data directories on the file system notesd's data directory lives on.
.PHONY: m1-evidence
m1-evidence: ## Record M1's exit evidence 1-4 (FROSTNOTES_OBSIDIAN_LINKS, FROSTNOTES_OBSIDIAN_VAULT)
	@test -n "$$FROSTNOTES_OBSIDIAN_LINKS" -a -n "$$FROSTNOTES_OBSIDIAN_VAULT" || { \
		echo "set FROSTNOTES_OBSIDIAN_LINKS to the dump and FROSTNOTES_OBSIDIAN_VAULT to the vault"; exit 2; }
	go test -tags integration -count=1 -v -run '^TestI(ObsidianLinkDump|ColdIndexOfObsidianVault)$$' ./internal/markdown ./internal/vault
	go test -tags integration -count=1 -v -run '^TestIVimSaveIsSearchable$$' ./internal/engine
	$(MAKE) --no-print-directory vault-it

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
	go test -race -timeout 25m -run '^Test[^I]' ./...

vulncheck: ## Report reachable vulnerabilities (govulncheck; needs the network)
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

clean: ## Remove build output
	rm -rf build dist coverage.out coverage.html

# The app (M0 Phase 4). Its toolchain runs in the nsl machine
# (docs/adr/0009-development-environment-and-gates.md); CI, whose runner has
# the toolchain, runs the same targets with IN_NSL='sh -c'.
.PHONY: ui-check ui-test ui-vitest ui-fmt ui-dev app-build app-test app-dev app-run spike-cm6 spike-cm6-nsl web-build lodge-app lodge-package

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

web-build: ## Build the web app for lodge into build/web, base /app/notes/ (nsl)
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm exec tsc --noEmit && pnpm exec vite build --mode web --outDir ../build/web --emptyOutDir'
	cp app/src-tauri/icons/icon.svg build/web/icon.svg

# notes' install tree for lodge (docs/specs/lodge-app.md, What notesd
# installs): copy build/lodge/apps/notes to /usr/share/lodge/apps/notes.
lodge-app: web-build ## Lay out build/lodge/apps/notes: app.json at this version and web/
	rm -rf build/lodge/apps/notes && mkdir -p build/lodge/apps/notes
	cp -R build/web build/lodge/apps/notes/web
	sed 's/"version": "dev"/"version": "$(VERSION)"/' packaging/lodge/app.json > build/lodge/apps/notes/app.json

# The /usr layout (frostyard/core ADR-0058): notesd, notesctl, the manifest,
# the web build, the units and the sysusers.d entry, staged under
# LODGE_DESTDIR by packaging/lodge/stage.sh. Needs make build and make
# web-build first.
LODGE_DESTDIR ?= build/lodge-root
lodge-package: ## Stage notes' /usr tree into LODGE_DESTDIR (default build/lodge-root); run build and web-build first
	packaging/lodge/stage.sh '$(LODGE_DESTDIR)' '$(VERSION)' build

# The release .deb (frostyard/core ADR-0011, ADR-0012, ADR-0058). Snapshot
# builds are versioned 0.0.0-COMMIT-snapshot; the release workflow passes
# DEB_VERSION=X.Y.Z, the tag without its v. The tree is staged from scratch
# by packaging/lodge/stage.sh, so app.json carries DEB_VERSION. Needs
# make web-build first (nsl).
DEB_VERSION ?= 0.0.0-$(COMMIT)-snapshot
DEB_PKG := frostyard-notesd
DEB_FILE := dist/$(DEB_PKG)_$(DEB_VERSION)_amd64.deb
DEB_LDFLAGS := -s -w -X main.version=$(DEB_VERSION) -X main.commit=$(COMMIT)

deb: ## Build dist/frostyard-notesd_<DEB_VERSION>_amd64.deb with nFPM, after the web build
	@test -f build/web/index.html -a -f build/web/icon.svg || { echo "build/web is missing: run make web-build first (nsl)"; exit 1; }
	rm -rf build/deb
	mkdir -p build/deb/bin dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '$(DEB_LDFLAGS)' -o build/deb/bin/ ./cmd/notesd ./cmd/notesctl
	cp -R build/web build/deb/bin/web
	packaging/lodge/stage.sh build/deb/root '$(DEB_VERSION)' build/deb/bin
	chmod -R u=rwX,go=rX build/deb/root
	DEB_VERSION='$(DEB_VERSION)' nfpm package -p deb -f packaging/nfpm.yaml -t dist/
	$(MAKE) --no-print-directory deb-check DEB_VERSION='$(DEB_VERSION)'

deb-check: ## Check the .deb's control fields, paths, owners, modes and payload
	packaging/deb-check.sh '$(DEB_FILE)' '$(DEB_PKG)' '$(DEB_VERSION)'

svu-version-check:
	@test -n "$(SVU_VERSION)" || { echo "mise.toml pins no svu"; exit 1; }
	@if ! command -v "$(SVU)" >/dev/null 2>&1; then \
		echo "svu $(SVU_VERSION) is required for make bump (not installed; run: mise install)"; \
		exit 1; \
	fi; \
	installed="$$($(SVU) --version 2>&1 | sed -n 's/^GitVersion:[[:space:]]*//p' | head -1)"; \
	if [ "$$installed" != "$(SVU_VERSION)" ]; then \
		echo "expected svu $(SVU_VERSION), found $${installed:-unknown} (run: mise install)"; \
		exit 1; \
	fi

# The only sanctioned tagging path (frostyard/core ADR-0012): the tag starts
# the release workflow.
bump: ## Tag the next version with svu from a clean main equal to origin/main, then push the tag
	@$(MAKE) --no-print-directory svu-version-check
	@test "$$(git rev-parse --abbrev-ref HEAD)" = main || { echo "make bump runs on main"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "Working directory is not clean: commit or stash first"; exit 1; }
	git fetch origin main
	@test "$$(git rev-parse HEAD)" = "$$(git rev-parse origin/main)" || { echo "HEAD is not origin/main: pull or push first"; exit 1; }
	$(MAKE) check
	@test -z "$$(git status --porcelain)" || { echo "make check changed the tree: commit that first"; exit 1; }
	@version=$$($(SVU) next); \
		git tag -a "$$version" -m "Version $$version"; \
		echo "Tagged $$version; pushing it to origin..."; \
		git push origin "$$version"

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

# M3's exit evidence (docs/plans/0005-m3-desktop-app.md, Phase 7) with the
# built app: 1, typing and scrolling in a generated 10,000-line note with
# every live-preview provider, on the host's GPU and under Xvfb in nsl;
# 2 and 3, every hotkey and the writer pairs, in nsl under Xvfb, where
# xdotool presses the keys (internal/appevidence).
.PHONY: m3-evidence
m3-evidence: ## Record M3's exit evidence 1-3 with build/frostnotes-app (from make app-build)
	@test -x build/frostnotes-app || { echo "build/frostnotes-app is missing; run make app-build first"; exit 1; }
	FROSTNOTES_SPIKE=editor build/frostnotes-app
	$(IN_NSL) 'FROSTNOTES_SPIKE=editor $(XVFB) build/frostnotes-app'
	$(IN_NSL) 'FROSTNOTES_APP=$$PWD/build/frostnotes-app $(XVFB) go test -tags integration -count=1 -timeout 15m -v -run "^TestI" ./internal/appevidence'

# M4's exit evidence 1 and 2 on loopback (docs/plans/0006-m4-notes-inside-lodge.md,
# Phases 5 and 7): lodge's development build at LODGE_E2E_REF
# (scripts/lodge-e2e.sh), notes' web build in it, Firefox ESR through
# geckodriver at a desktop size and at 390 x 844, and for evidence 2 the
# desktop app under Xvfb.
LODGE_E2E_REF := c48d43acc70038ff07100c8f816de387570bd817

.PHONY: lodge-e2e m4-evidence
lodge-e2e: ## Build lodge's development build at LODGE_E2E_REF into build/lodge-e2e (nsl)
	$(IN_NSL) 'scripts/lodge-e2e.sh $(LODGE_E2E_REF) build/lodge-e2e'

m4-evidence: lodge-app lodge-e2e ## Record M4's exit evidence 1 and 2 on loopback (nsl; run make app-build first)
	@test -x build/frostnotes-app || { echo "build/frostnotes-app is missing; run make app-build first"; exit 1; }
	$(IN_NSL) 'M4_EVIDENCE_DIR=$$PWD/build/m4-evidence LODGE_E2E=$$PWD/build/lodge-e2e NOTES_WEB=$$PWD/build/lodge/apps/notes FROSTNOTES_APP=$$PWD/build/frostnotes-app $(XVFB) go test -tags integration -count=1 -timeout 20m -v -run "^TestI" ./internal/webevidence'

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
