.PHONY: build install serve close htcli-build htcli-install htcli-clean \
	ext-build ext-dev ext-zip firefox-build firefox-install firefox-zip

-include .env.local
export

HTCLI_DIR := htcli

# ── htcli ──────────────────────────────────────────────────────────────

htcli-build:
	cd $(HTCLI_DIR) && go build -o bin/htcli ./cmd/htcli

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

build: htcli-build ext-build

install: htcli-install
	@if [ -n "$(EXTID)" ]; then \
		echo "Registering native host for extension $(EXTID)..."; \
		htcli install --extension-id "$(EXTID)"; \
	else \
		echo "htcli installed. Set EXTID in .env or run: htcli install --extension-id <ID>"; \
	fi

serve:
	htcli serve

close:
	@lsof -ti :3845 | xargs kill -9 2>/dev/null && echo "Killed process on :3845" || echo "Nothing running on :3845"

list:
	HTR_BEARER_TOKEN=htr_aia_2026 htcli tabs list

