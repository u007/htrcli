---
name: cdp-integration-keychain-hang
description: htrcli CDP integration tests (temp HOME) hang on any network load on macOS once a keychain dialog appears; run with a --use-mock-keychain Chrome wrapper and sandbox disabled
metadata:
  type: project
---

`go test -tags integration ./internal/cdp/` sets a temp `HOME`; on macOS headless Chrome then can't reach the login keychain, the OS pops a dialog, and every http(s) load in the test browser hangs in `Page.navigate` (about:blank-only tests still pass). Seen 2026-10-07.

**Why:** Chrome derives cookie-encryption keys from the keychain located via $HOME.

**How to apply:** run integration tests with `HTRCLI_CHROME_PATH=<script that execs Chrome with --use-mock-keychain "$@">`, and with the Bash sandbox disabled (sandbox blocks localhost to the debugging port). A timed-out run leaks Chrome on its port; `pkill -9 -f remote-debugging-port=922x` before rerunning, else the next run adopts the hung browser.
