.PHONY: build install serve close \
		htcli-build htcli-build-all htcli-install htcli-clean \
		ext-build ext-dev ext-zip firefox-build firefox-install firefox-zip

-include .env.local
export

HTCLI_DIR := htcli

# ── htcli ──────────────────────────────────────────────────────────────

htcli-build:
	cd $(HTCLI_DIR) && go build -o bin/htcli ./cmd/htcli

# Cross-compile htcli for all supported OS/arch combinations.
# Binaries are placed in htcli/bin/ with platform-specific names.
htcli-build-all:
	cd $(HTCLI_DIR) && \
	GOOS=darwin  GOARCH=amd64 go build -o bin/htcli-darwin-amd64   ./cmd/htcli && \
	GOOS=darwin  GOARCH=arm64 go build -o bin/htcli-darwin-arm64   ./cmd/htcli && \
	GOOS=linux   GOARCH=amd64 go build -o bin/htcli-linux-amd64    ./cmd/htcli && \
	GOOS=linux   GOARCH=arm64 go build -o bin/htcli-linux-arm64    ./cmd/htcli && \
	GOOS=windows GOARCH=amd64 go build -o bin/htcli-windows-amd64.exe ./cmd/htcli

htcli-install:
	cd $(HTCLI_DIR) && go install ./cmd/htcli

htcli-clean:
	rm -rf $(HTCLI_DIR)/bin

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

# ── Combined ───────────────────────────────────────────────────────────

build: htcli-build-all ext-build firefox-build

install: htcli-install
	@if [ -n "$(EXTID)" ]; then \
		echo "Registering native host for extension $(EXTID)..."; \
		htcli install --extension-id "$(EXTID)"; \
	else \
		echo "htcli installed. Set EXTID in .env or run: htcli install --extension-id <ID>"; \
	fi

serve:
	@port=$${HTR_PORT:-3845}; \
	pid=$$(netstat -anv -p tcp 2>/dev/null | awk -v port="$$port" '$$6 == "LISTEN" {split($$4,addr,"."); if (addr[length(addr)] == port) {split($$11,a,":"); print a[2]; exit}}'); \
	if [ -n "$$pid" ]; then \
		echo "Killing process $$pid on port $$port..."; \
		kill -9 $$pid 2>/dev/null || true; \
	fi; \
	htcli serve

close:
	@port=$${HTR_PORT:-3845}; \
	pid=$$(netstat -anv -p tcp 2>/dev/null | awk -v port="$$port" '$$6 == "LISTEN" {split($$4,addr,"."); if (addr[length(addr)] == port) {split($$11,a,":"); print a[2]; exit}}'); \
	if [ -n "$$pid" ]; then \
		kill -9 $$pid 2>/dev/null && echo "Killed process on :$$port" || echo "Failed to kill on :$$port"; \
	else \
		echo "Nothing running on :$$port"; \
	fi

list:
	HTR_BEARER_TOKEN=htr_aia_2026 htcli tabs list
