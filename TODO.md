# TODO

## htrcli network capture — deferred / known limitations

- Response bodies are captured on Chrome (CDP getResponseBody, 64KB cap) but NOT on Firefox (webRequest cannot cheaply read bodies). Firefox network entries are metadata-only.
- The shared debuggerManager is used only by network + dialog capture. The existing trusted-input (cdpInput.ts) and CDP_EVAL paths still do their own chrome.debugger.attach, so running a click/eval command while a capture window is open on the same tab fails with "Already attached". Route those through debuggerManager in a later pass.
- Network mocking/interception (spec §1b) is a separate deferred phase (needs webRequestBlocking + Fetch.enable).

## From review-changes (2026-07-09)

Plan gaps surfaced during the review of the Playwright-parity change set.

- [ ] Part 3: `keyMap.ts` with `resolveKey` and full test coverage (from review-changes: 2026-07-09)
- [ ] Part 3: `prepareClick` / `prepareKeys` content-script actions (from review-changes: 2026-07-09)
- [ ] Part 3: Background CDP dispatchers (`dispatchCdpClick`/`dispatchCdpKey`/`dispatchCdpType`) and routing gate in `sendCommandToTab` (from review-changes: 2026-07-09)
- [ ] Part 3: Firefox synthetic-event upgrade (pointer events) for click/pressKey/type (from review-changes: 2026-07-09)
- [ ] Part 3: WS-path relay via `chrome.runtime.sendMessage` and three new `MessageType` entries (from review-changes: 2026-07-09)
