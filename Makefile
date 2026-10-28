.PHONY: build install serve close bench-smoke bench-office bench-tasks \
		bench-form \
		bench-booking \
		bench-expense \
		bench-files \
		bench-mail \
		bench-crm \
		bench-messaging \
		bench-tickets \
		bench-leave \
		bench-social \
		bench-survey \
		bench-finance \
		bench-store \
		bench-research \
		htrcli-build htrcli-build-all htrcli-build-linux htrcli-install htrcli-clean \
		ext-build ext-dev ext-zip firefox-build firefox-install firefox-zip

-include .env
-include .env.local
export

HTRCLI_DIR := htrcli

# HTRCLI_VERSION is stamped into commands.Version; "dev" when git cannot describe.
HTRCLI_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
VERSION_LDFLAGS := -ldflags "-X github.com/u007/htrcli/internal/commands.Version=$(HTRCLI_VERSION)"

# ── htrcli ──────────────────────────────────────────────────────────────

htrcli-build:
	cd $(HTRCLI_DIR) && go build $(VERSION_LDFLAGS) -o bin/htrcli ./cmd/htrcli

# Cross-compile htrcli for the targets that build from any host.
# Binaries are placed in htrcli/bin/ with platform-specific names.
#
# Targets requiring a native toolchain are built on their own OS:
#   - darwin  : needs the macOS SDK (build on a macOS host)
#   - linux   : needs libappindicator dev headers; use `make htrcli-build-linux`
# Windows cross-compiles via the default stub tray backend
# (internal/tray/tray_real_windows.go); pass `-tags htrcli_native_tray` to
# build it with the real getlantern/systray backend instead.
htrcli-build-all:
	cd $(HTRCLI_DIR) && \
	GOOS=darwin  GOARCH=arm64 go build $(VERSION_LDFLAGS) -o bin/htrcli-darwin-arm64   ./cmd/htrcli && \
	GOOS=windows GOARCH=amd64 go build $(VERSION_LDFLAGS) -o bin/htrcli-windows-amd64.exe ./cmd/htrcli

# Native Linux build (CGO + libappindicator dev headers required, e.g.
# `apt-get install libayatana-appindicator3-dev`). Built for the host
# architecture; pass GOARCH with a matching cross C toolchain for others.
htrcli-build-linux:
	cd $(HTRCLI_DIR) && go build $(VERSION_LDFLAGS) -o bin/htrcli-linux-$$(go env GOARCH) ./cmd/htrcli

htrcli-install:
	cd $(HTRCLI_DIR) && go install $(VERSION_LDFLAGS) ./cmd/htrcli

htrcli-clean:
	rm -rf $(HTRCLI_DIR)/bin

# Run the gated CDP end-to-end integration test (requires a real Chrome).
htrcli-test-integration:
	cd $(HTRCLI_DIR) && go test -tags integration ./internal/cdp/ -run TestCDPEndToEnd -v

# Benchmark targets run one or more scenario files over the CDP transport
# against LOCAL fixture pages in docs/benchmarks/fixtures/ (requires a real
# Chrome; no authenticated session and no real data is touched). Reports are
# written under docs/benchmarks/results/.
BENCH_FIXTURES := $(CURDIR)/docs/benchmarks/fixtures

# $(1): fixture env assignments, $(2): scenario file under
# docs/benchmarks/scenarios/, $(3): report name prefix.
define bench_run
	cd $(HTRCLI_DIR) && \
		$(1) \
		./bin/htrcli bench run \
		../docs/benchmarks/scenarios/$(2) \
		--transport cdp \
		--out ../docs/benchmarks/results/$(3)-$$$$.json
endef

# Offline smoke scenario.
bench-smoke: htrcli-build
	$(call bench_run,BENCH_FIXTURE="$(BENCH_FIXTURES)/smoke.html",s01-offline-smoke.json,smoke)

# Office-app fixture scenarios.
bench-office: htrcli-build
	$(call bench_run,BENCH_FIXTURE_GMAIL="$(BENCH_FIXTURES)/gmail.html" BENCH_FIXTURE_CALENDAR="$(BENCH_FIXTURES)/calendar.html" BENCH_FIXTURE_DOCS="$(BENCH_FIXTURES)/docs.html" BENCH_FIXTURE_SHEETS="$(BENCH_FIXTURES)/sheets.html",s02-gmail-fixture.json,office-gmail)
	$(call bench_run,BENCH_FIXTURE_CALENDAR="$(BENCH_FIXTURES)/calendar.html" BENCH_FIXTURE_DOCS="$(BENCH_FIXTURES)/docs.html" BENCH_FIXTURE_SHEETS="$(BENCH_FIXTURES)/sheets.html",s03-office-fixtures.json,office-apps)

# Tasks/reminder fixture scenario.
bench-tasks: htrcli-build
	$(call bench_run,BENCH_FIXTURE_TASKS="$(BENCH_FIXTURES)/tasks.html",s04-tasks-fixture.json,tasks)

bench-form: htrcli-build
	$(call bench_run,BENCH_FIXTURE_FORM="$(BENCH_FIXTURES)/form.html",s09-form-fixture.json,form)

bench-booking: htrcli-build
	$(call bench_run,BENCH_FIXTURE_BOOKING="$(BENCH_FIXTURES)/booking.html",s10-booking-fixture.json,booking)

bench-expense: htrcli-build
	$(call bench_run,BENCH_FIXTURE_EXPENSE="$(BENCH_FIXTURES)/expense.html" BENCH_FIXTURE_RECEIPT="$(BENCH_FIXTURES)/receipt.txt",s11-expense-fixture.json,expense)

bench-files: htrcli-build
	$(call bench_run,BENCH_FIXTURE_FILES="$(BENCH_FIXTURES)/files.html" BENCH_FIXTURE_REPORT="$(BENCH_FIXTURES)/report.txt",s12-files-fixture.json,files)

bench-mail: htrcli-build
	$(call bench_run,BENCH_FIXTURE_MAIL="$(BENCH_FIXTURES)/mail.html",s13-mail-actions.json,mail)

bench-crm: htrcli-build
	$(call bench_run,BENCH_FIXTURE_CRM="$(BENCH_FIXTURES)/crm.html",s14-crm-fixture.json,crm)

bench-messaging: htrcli-build
	$(call bench_run,BENCH_FIXTURE_MESSAGING="$(BENCH_FIXTURES)/messaging.html",s15-messaging-fixture.json,messaging)
	$(call bench_run,BENCH_FIXTURE_MESSAGING="$(BENCH_FIXTURES)/messaging.html",s17-thread-replies.json,messaging-threads)

bench-tickets: htrcli-build
	$(call bench_run,BENCH_FIXTURE_TICKET="$(BENCH_FIXTURES)/ticket.html" BENCH_FIXTURE_REPORT="$(BENCH_FIXTURES)/report.txt",s18-ticket-fixture.json,tickets)

bench-leave: htrcli-build
	$(call bench_run,BENCH_FIXTURE_LEAVE="$(BENCH_FIXTURES)/leave.html",s19-leave-fixture.json,leave)

bench-social: htrcli-build
	$(call bench_run,BENCH_FIXTURE_SOCIAL="$(BENCH_FIXTURES)/social.html" BENCH_FIXTURE_REPORT="$(BENCH_FIXTURES)/report.txt",s20-social-fixture.json,social)

bench-survey: htrcli-build
	$(call bench_run,BENCH_FIXTURE_SURVEY="$(BENCH_FIXTURES)/survey.html",s21-survey-fixture.json,survey)

bench-finance: htrcli-build
	$(call bench_run,BENCH_FIXTURE_FINANCE="$(BENCH_FIXTURES)/finance.html",s22-finance-fixture.json,finance)

bench-store: htrcli-build
	$(call bench_run,BENCH_FIXTURE_STORE="$(BENCH_FIXTURES)/store.html",s23-store-fixture.json,store)

bench-research: htrcli-build
	$(call bench_run,BENCH_FIXTURE_RESEARCH_A="$(BENCH_FIXTURES)/research-source-a.html" BENCH_FIXTURE_RESEARCH_B="$(BENCH_FIXTURES)/research-source-b.html" BENCH_FIXTURE_RESEARCH_SUMMARY="$(BENCH_FIXTURES)/research-summary.html",s16-research-fixture.json,research)

# ── Extension ──────────────────────────────────────────────────────────

ext-build:
	bun run build

ext-dev:
	bun run dev

ext-zip:
	bun run zip

firefox-build:
	bun run firefox:build

firefox-zip:
	bun run firefox:zip

firefox-install: firefox-build
	@PROFILE_DIR="$${FIREFOX_PROFILE:-/tmp/ff-webext-profile}"; \
	mkdir -p "$$PROFILE_DIR"; \
	bunx web-ext run --source-dir firefox/build \
		--firefox="$${FIREFOX_BIN:-/Applications/Firefox.app/Contents/MacOS/firefox}" \
		--firefox-profile "$$PROFILE_DIR" \
		--no-input
	htrcli install --browser firefox --extension-id htrncontrol@mercstudio.com

# ── Combined ───────────────────────────────────────────────────────────

build: htrcli-build-all ext-build firefox-build

install: htrcli-install
	@if [ -n "$(EXTID)" ]; then \
		echo "Registering native host for extension $(EXTID)..."; \
		htrcli install --extension-id "$(EXTID)"; \
	else \
		echo "htrcli installed. Set EXTID in .env or run: htrcli install --extension-id <ID>"; \
	fi

serve:
	@port=$${HTR_PORT:-3845}; \
	pid=$$(netstat -anv -p tcp 2>/dev/null | awk -v port="$$port" '$$6 == "LISTEN" {split($$4,addr,"."); if (addr[length(addr)] == port) {split($$11,a,":"); print a[2]; exit}}'); \
	if [ -n "$$pid" ]; then \
		echo "Killing process $$pid on port $$port..."; \
		kill -9 $$pid 2>/dev/null || true; \
	fi; \
	htrcli serve

close:
	@port=$${HTR_PORT:-3845}; \
	pid=$$(netstat -anv -p tcp 2>/dev/null | awk -v port="$$port" '$$6 == "LISTEN" {split($$4,addr,"."); if (addr[length(addr)] == port) {split($$11,a,":"); print a[2]; exit}}'); \
	if [ -n "$$pid" ]; then \
		kill -9 $$pid 2>/dev/null && echo "Killed process on :$$port" || echo "Failed to kill on :$$port"; \
	else \
		echo "Nothing running on :$$port"; \
	fi

list:
	HTR_BEARER_TOKEN=htr_aia_2026 htrcli tabs list
