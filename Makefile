.PHONY: build install serve close \
		htrcli-build htrcli-build-all htrcli-install htrcli-clean \
		ext-build ext-dev ext-zip firefox-build firefox-install firefox-zip

-include .env
-include .env.local
export

HTRCLI_DIR := htrcli

# ── htrcli ──────────────────────────────────────────────────────────────

htrcli-build:
	cd $(HTRCLI_DIR) && go build -o bin/htrcli ./cmd/htrcli

# Cross-compile htrcli for all supported OS/arch combinations.
# Binaries are placed in htrcli/bin/ with platform-specific names.
htrcli-build-all:
	cd $(HTRCLI_DIR) && \
	GOOS=darwin  GOARCH=amd64 go build -o bin/htrcli-darwin-amd64   ./cmd/htrcli && \
	GOOS=darwin  GOARCH=arm64 go build -o bin/htrcli-darwin-arm64   ./cmd/htrcli && \
	GOOS=linux   GOARCH=amd64 go build -o bin/htrcli-linux-amd64    ./cmd/htrcli && \
	GOOS=linux   GOARCH=arm64 go build -o bin/htrcli-linux-arm64    ./cmd/htrcli && \
	GOOS=windows GOARCH=amd64 go build -o bin/htrcli-windows-amd64.exe ./cmd/htrcli

htrcli-install:
	cd $(HTRCLI_DIR) && go install ./cmd/htrcli

htrcli-clean:
	rm -rf $(HTRCLI_DIR)/bin

# Run the gated CDP end-to-end integration test (requires a real Chrome).
htrcli-test-integration:
	cd $(HTRCLI_DIR) && go test -tags integration ./internal/cdp/ -run TestCDPEndToEnd -v

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
