---
name: browser-smoke-port-9222-busy
description: The user's own Chrome often listens on 9222; run htrcli browser smoke tests on HTRCLI_CDP_PORT=9333 and never kill it
metadata:
  type: reference
---

A non-htrcli Google Chrome (remote debugging on 127.0.0.1:9222, no ~/.htrcli/browser.json) is usually running on this Mac. `curl` to 9222 then returns 404 on `/` and the process is not htrcli's.

**Why:** the plan says to report a Chrome already on the CDP port instead of killing it, and `browser start` on 9222 would adopt that listener and write its PID into browser.json.

**How to apply:** smoke `browser start/status/stop` with `HTRCLI_CDP_PORT=9333` (check the port is free with lsof first). Use `browser status --json` to see the recorded args. The user's configured chrome-path is a wrapper script, so the Chrome argv includes its extra `--ignore-certificate-errors`.
