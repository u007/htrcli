# TODO

## htrcli network capture — deferred / known limitations

- Response bodies are captured on Chrome (CDP getResponseBody, 64KB cap) but NOT on Firefox (webRequest cannot cheaply read bodies). Firefox network entries are metadata-only.
- FIXED 2026-10-10: `cdpInput.ts` `runWithDebugger` now routes through `debuggerManager.attachShared`/`detachShared` (refcounted, not direct `chrome.debugger.attach`). `debuggerManager.test.ts` confirms a second attach during an open capture does not throw. A live `dispatchCdpClick` during open capture is the remaining verification step.
- Network mocking/interception (spec §1b) is a separate deferred phase (needs webRequestBlocking + Fetch.enable).

## htrcli console/network over --cdp — known limitations (2026-10-07)

- `network read --cdp` reads the Performance API, so already-completed requests have no method and no request/response bodies; in-flight requests are absent and the resource-timing buffer (250 by default) can drop later entries. Recovering these would need a long-lived CDP attachment (daemon-side buffer), which `--cdp` deliberately does not have.
- `network wait --cdp` sees only requests completing after it attaches; callers must start it before the triggering action (documented in README/SKILL).
- `console read --cdp` gets V8's replay without object previews (objects print as `Object`); browser-side `Log` domain entries (e.g. "Failed to load resource") are not collected.
- `--since` is rejected over `--cdp`: seq numbers are per-invocation ordinals.
- Listings are not paginated on either transport (no `--limit`/`--offset` on `console`/`network read`).

## htrcli pick / fill-form / --frame — deferred (2026-10-07)

- `--frame` is one level and same-origin only. Nested frames (repeat `--frame`) and cross-origin/out-of-process iframes (would need `Target.attachToTarget` on the frame's own target) are not supported; the cross-origin case errors with an `open <src>` hint.
- `--frame` is rejected by verbs that do not carry the `htrcli.frame-aware` cobra annotation (`findAll`, `hover`, `press`, `upload`, `drag`, `mousedown/up/move`, `eval`, `screenshot`, …), and `@eN` refs / `xy=` / `find --ref` cannot be combined with it. To honour `--frame` in a new verb, annotate its `cobra.Command` with `FrameAwareAnnotations()` — there is deliberately no name-keyed allowlist to keep in sync.
- Resolved 2026-10-08: cross-realm args in `--frame` scoped calls — `scopedExpr` now ships the args as a JSON string and parses them with the frame realm's own `JSON.parse`, so the objects a frame function receives carry frame-realm prototypes (pinned by `TestScopedExpr`).
- `fill-form` rejects a selector that matches several elements (incl. a whole checkbox/radio group); a "choose radio by value" form is not implemented.
- Resolved 2026-10-07: `pick` types the full requested text as the query (decision: "type the full text"; no `--query` flag).
- Integration tests set a temp `HOME`, and on macOS headless Chrome then cannot reach the login keychain: once the OS shows its keychain dialog, every network load in the test browser hangs (seen 2026-10-07; `about:blank`-only `TestCDPEndToEnd` is unaffected). Workaround: run with `HTRCLI_CHROME_PATH` pointing at a wrapper that execs Chrome with `--use-mock-keychain`. A proper fix needs a test-only launch-args hook in `StartBrowser`.

## htrcli dialog handling — known limitations

- The Firefox MAIN-world override is racy against dialogs a page fires before it lands at document_start, and cannot intercept native beforeunload dialogs (Chrome CDP handles beforeunload; Firefox does not).
- Dialog handling shares the refcounted debuggerManager with network capture; the "Already attached" limitation from the network plan's TODO applies here too.

## Low-level input primitives — known limitations (2026-08-24)

- Synthetic drag (`mousedown`/`mousemove`/`drag`/`mouseup` on Firefox) dispatches
  pointer/mouse events only. Native HTML5 DnD (`dragstart`/`dragover`/`drop` with
  `DataTransfer`) is NOT synthesized. Custom sortables/sliders that listen to
  pointer/mouse events work; native file-drop zones and `draggable=true` DnD
  require `eval` with a manual `DataTransfer`. Live Firefox DnD smoke (native
  `draggable` widget) has not been run. See `htrcli/README.md` note and
  `src/contentScript/commandExecutor.ts:handleDrag`.
- `keydown`/`keyup` are stateless per-command. Holding a modifier across several
  commands is caller-managed (`keydown Shift` -> ... -> `keyup Shift`); modifiers
  are passed per `Input.dispatchKeyEvent` call, no daemon-side held-key tracking.
- Synthetic viewport-coordinate mouse input uses `document.elementFromPoint` for
  hit-testing and returns an explicit error when no element is under the point;
  CDP `@eN` drag endpoints resolve through the persistent backend-node ref store
  and reach the page-side drag resolver as element handles (scrolled and read
  together with the other endpoint), never as pre-computed pixels.
- Drag steps and delay are bounded to `1..100` and `0..2000ms` respectively;
  values outside those ranges are clamped.

## From review-changes (2026-07-09)

Plan gaps surfaced during the review of the Playwright-parity change set.

- [ ] Part 3: `keyMap.ts` with `resolveKey` and full test coverage (from review-changes: 2026-07-09)
- [ ] Part 3: `prepareClick` / `prepareKeys` content-script actions (from review-changes: 2026-07-09)
- [ ] Part 3: Background CDP dispatchers (`dispatchCdpClick`/`dispatchCdpKey`/`dispatchCdpType`) and routing gate in `sendCommandToTab` (from review-changes: 2026-07-09)
- [ ] Part 3: Firefox synthetic-event upgrade (pointer events) for click/pressKey/type (from review-changes: 2026-07-09)
- [ ] Part 3: WS-path relay via `chrome.runtime.sendMessage` and three new `MessageType` entries (from review-changes: 2026-07-09)

## Review resolution — /review-changes (project f10d385e9683, 2026-09-13)

Scope (audit trail): working tree (`git diff HEAD` + 4 untracked files: htrcli/internal/cdp/process_unix.go, process_windows.go; internal/commands/record_process_unix.go, record_process_windows.go). Report relayed unmodified.
Note: Other working-tree changes (main.go, cdp/launch.go, host/bridge.go, host/relay.go, host/server.go, icon.png) are pre-existing from commit a86dfe7; this review fixed only serve.go and record.go.

Findings resolved:
- [Fixed] serve.go:90 — restored `syscall.SIGTERM` to `signal.Notify`; added `syscall` import.
- [Fixed] record.go:222 — restored `syscall.SIGTERM` to `signal.Notify`; added `syscall` import.

Findings skipped:
- icon.png binary change (79 -> 411308 bytes) — SKIPPED (intentional per user).

Plan gaps recorded:
- No explicit plan/spec file for the "low-level input primitives" change (commit a86dfe7); verified via commit message + source inspection.

Verification performed:
- `gofmt`, `go test ./...`, `go vet ./...`, `make htrcli-build` — all clean.
- `make htrcli-build-all` FAILS (systray undefined: nativeLoop, registerSystray, etc.); cross-build not validated, native build only.

Breaking change register: SIGTERM graceful-shutdown restored (was broken). No new breaking changes.

## Review resolution — code review (project f10d385e9683, 2026-09-18)

Scope (audit trail): working tree `git diff HEAD` + untracked htrcli platform files. Reviewed 12 modified + 5 untracked files.

Findings resolved (5 warnings):
- [Fixed] `record_process_unix.go` — `recordTerminateProcess` now ignores `ESRCH` like `cdp.terminatePID`, closing the check-then-signal race.
- [Fixed] `tray_real_windows.go` — stub now documented as the default Windows backend and gated behind the inverse of the new `htrcli_native_tray` tag; `tray_real.go` accepts `windows && htrcli_native_tray` so native builds get the real tray.
- [Fixed] `process_windows.go` / `record_process_windows.go` — `powershell` CIM is now the primary `CommandLine` source with a `wmic` fallback (wmic removed on Win11 24H2+).
- [Fixed] `process_windows.go` / `record_process_windows.go` — `processAlive` parses `tasklist /FO CSV` and compares the PID column, removing the English-localized message dependency.
- [Fixed] `Makefile` — deduped the build-all comment, documented native-only targets, and added `make htrcli-build-linux`.

Findings deferred:
- [Open] Critical security: Windows relay binds `127.0.0.1:3847` and greets with the bearer token before auth; any local process can obtain it. Needs a per-user named pipe or relay-handshake auth (`htrcli/internal/host/bridge.go:23-29`, `relay.go:18-28`).
- [Open] Suggestions 1-4 from the review (redundant native-host detection condition, misleading `/api/health` socket field, hardcoded relay port, process-shim duplication).

Verification performed:
- `gofmt -l` (only pre-existing `internal/commands/publish.go`), `go build ./...`, `go vet ./...`, `go test ./...` — clean.
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build` — clean (default stub).
- `make -n htrcli-build-all` / `make -n htrcli-build-linux` — recipe resolves.

## Session recordings — known limits and deferred work (2026-09-29)

Deliberate limits (documented in `skills/htrcli/SKILL.md` + `htrcli/README.md`, not bugs):
- `--with-screenshots` / `export` pre-check the step count from `list` and refuse
  over `maxMediaSteps` (120). The real limit is BYTES: one native-messaging frame
  capped at `host.MaxMessageSize` (64 MiB), and base64 inflates PNGs ~4/3, so ~48 MiB
  of raw screenshots is already over. The step count is a proxy because the
  extension cannot report a size before serialising.
- The preflight only inspects the first `maxMediaSteps+1` sessions, so a fetch of an
  older session is not pre-checked (it is allowed through, which is the safe
  direction).



## Tab-less background commands — deferred / closed as not-worth-doing

Closed (decided against, with reasoning, so they are not re-proposed):

- **Streaming base64→deflate during `export`.** Considered and rejected: the
  saving is one `[]byte` per media blob, and the payload is already fully parsed
  in memory by the time encoding could begin, so it would not change the peak
  footprint on the ~48 MB case that motivated it. The single-pass decode
  (`ExecuteCommandInto` / `ExecuteBackgroundCommandInto`) already removed the
  extra copies that were worth removing, and is pinned by an allocation bound.
- **Strict `--browser` selection (error instead of fallback).** Rejected in
  favour of first-wins fallback. A hint naming a profile that is not connected
  is a recoverable situation — `recordings list` prints `Answered by: <browser>`,
  so the user can see which profile served the request. Hard-failing would be
  strictly worse. If that ever needs revisiting, the change is confined to
  `Daemon.selectConn` in `htrcli/internal/host/daemon.go`.

Still open:

- **No React component tests.** The repo has no component-test harness and the
  user declined adding one, so the popup's new delete button and the side-panel
  recording toggle are covered only by their pure logic. A regression that broke
  rendering rather than behaviour would not be caught.
- **`identify` is not authenticated.** It arrives over the native-messaging
  relay, which the OS already scopes to the installed extension, so this is not
  an exposure today. It would become one if a relay were ever exposed over a
  socket. The daemon validates the value against the browsers it knows and
  ignores anything else, which bounds the blast radius.
- **Two connections announcing the same browser resolve to the earliest.** That
  is deterministic and documented, but a caller that genuinely wants the *other*
  chrome profile has no way to ask. Needs a profile label from the extension
  (e.g. the profile directory) before it can be addressed.
- **`--browser` without `--tab` is still ignored.** The root flag reaches a
  tab-routed command only when `--tab` is given, and then it is strict. Without
  `--tab` the command takes the earliest-connected tab, because the daemon
  rejects a browser on the untargeted route. Worth rejecting or routing by
  browser if it starts confusing users.
- **`--browser` strictness covers commands, upload, and tab lookup only.**
  `GET /api/page`, `GET /api/screenshot`, and the events endpoints still pick the
  earliest-connected owner of a shared tab ID and ignore `--browser`. Thread the
  hint through `GetPageInfo`, `GetScreenshotOpts`, and `GetEvents` the same way
  `ExecuteCommand` does.
- **Tab-routed `openTab` and `closeTab` still use the content-script path.**
  `htrcli inspect` closes and opens tabs through a tab-routed command, so it
  still needs a connected page in that browser. Only the tab-less route (htrcli
  `tabs new` and `tabs close`) runs in the background worker.

## Office-task benchmark work queue (docs/benchmarks/office-task-benchmark.md)

Work top-down; one item at a time. Each entry: task, why it matters, htrcli
gap, acceptance.

- [ ] B01 Find emails (Gmail) — P0. Gap: DOM `wait` + virtualized list extraction. Acceptance: structured list with sender/subject/date. (DEFERRED: needs live Gmail auth. `wait` DONE, `collect` DONE — only the live run remains.)
- [ ] B02 Reply to email — P0. Gap: contenteditable typing (`type`), signature preservation. Safety: reply to self / draft.
- [ ] B03 Arrange meeting (Calendar) — P0. Gap: date/time pickers, guest autocomplete `pick`. Acceptance: event visible, invites sent (test account).
- [x] B04 Create todo/reminder — P0. Gap: due-date widgets, quick-add vs fields. Acceptance: task with correct due date. DONE via the local Tasks fixture (docs/benchmarks/fixtures/tasks.html) + scenarios S07/S08 (docs/benchmarks/scenarios/s04-tasks-fixture.json, run with `make bench-tasks`): both the quick-add path (one field, committed with Enter) and the full-form path (a details panel whose date is a POPOVER grid of day buttons — no date field) work. The live Google Tasks run is still pending auth (see the SKIPPED line).
- [ ] B05 Google Docs create/edit/share — P1. Gap: canvas editor, keyboard-only. Safety: share with test account.
- [ ] B06 Sheets/Excel data + formula — P1. Gap: cell navigation, virtualized grid. Acceptance: formula result matches.
- [ ] B07 Booking — P1. Gap: date pickers, seat maps. Safety: test slot, cancel after capture; never enter card.
- [ ] B08 Forward/label/archive email — P1.
- [ ] B09 Expense report/invoice — P2. Gap: receipt upload, multi-page.
- [ ] B10 File management — P2.
- [ ] B11 Contact/CRM update — P2.
- [ ] B12 Messaging — P2.
- [ ] B13 Form filling — P2.
- [ ] B14 Research & summarize — P2.
- [ ] B15-B20 Occasional — P3.
- [x] Enhancement: DOM-level `wait` command (selector/text/URL) both transports (htrcli wait).
- [x] Enhancement: rich-text (contenteditable) typing helpers (handleFill/handleType use document.execCommand selectAll/delete/insertText + input/change/blur dispatch).
- [x] Enhancement: virtualized-list extraction helper (`htrcli collect` — scrolls the list container, accumulates + de-dupes rows; both transports).
- [x] Enhancement: retry-with-backoff wrapper (`htrcli retry -- <cmd>` with `--times`/`--delay`/`--max-delay`; opt-in, logs every failed attempt to stderr, non-zero exit on exhaustion; works on both transports since it re-invokes htrcli; a transport flag placed before `retry` is forwarded too). Tests: retry_test.go (5).
- [x] Enhancement: structured data extraction command (`htrcli extract table|list`; TS handlers + Go command + tests + bundle).
  - Chosen over retry-with-backoff because B01 acceptance (structured list with sender/subject/date) needs extraction first; collect returns raw element info, extract turns it into records. (Retry/backoff has since been added too — see the entry above.)
- [ ] SKIPPED (user choice, 2026-10-20): live B01 run against a real Gmail session. The `wait` and `collect` building blocks it depends on are done; the remaining work is only the authenticated run + metrics capture.
- [ ] SKIPPED (user choice, 2026-10-20): B02-B20 live benchmark runs — all need authenticated web sessions (Gmail, Calendar, Docs, Sheets). The reusable pieces they need (`wait`, `collect`, contenteditable typing) are implemented; each scenario's live run is tracked in its own line above.
- [x] Enhancement: skill doc updates per office app (Gmail B01/B02, Calendar B03, Docs B05, Sheets B06, Booking B07) — `skills/htrcli/SKILL.md` section "Office apps: Gmail, Calendar, Docs, Sheets", each recipe verified against the local fixtures in `docs/benchmarks/fixtures/` (no real Google data touched). Runnable scenarios: s02-gmail-fixture.json (S02/S03) and s03-office-fixtures.json (S04/S05/S06) — all PASS over CDP via `make bench-office`.
- [x] FIX: `type` on a contenteditable APPENDED nothing safe — it did selectAll+delete+insertText, so it WIPED a Gmail reply box's quoted thread and signature. Now it places the caret at the end and appends; `fill` keeps its replace semantics (correct for a spreadsheet cell). Tests: src/contentScript/contentEditableTyping.test.ts (3) + benchmark scenario S03. Mutation-verified both ways.
- [x] FIX: `pick` and `fill-form` were COMPLETELY BROKEN over --cdp — the embedded formkit.js ends with `});`, and the installer wrapped it as `window.__htrcliFormKit = ( ... )`, which is a syntax error ("Unexpected token ';'"). Every CDP pick/fill-form failed with "installing form kit". Fixed in htrcli/internal/cdp/pick.go (trim the trailing semicolon). Tests: htrcli/internal/cdp/pick_install_test.go (2, mutation-verified). Pre-existing bug, not from this work.
- [x] FIX: `check`/`uncheck` failed on ARIA checkboxes — "Element is not a checkbox/radio input". Google Tasks, Material UI and most modern design systems render a checkbox as `<div role="checkbox" aria-checked>`, not an `<input type=checkbox>`. `check`/`uncheck` now drive aria-checked and dispatch a click so the app's own handler runs; the click comes FIRST (an app that toggles would otherwise flip the pre-set state back), and the attribute is enforced afterwards. Idempotent, and a tri-state (`aria-checked="mixed"`) is no longer a silent no-op. Tests: src/contentScript/ariaCheckbox.test.ts (8, incl. a click-count assertion) + benchmark scenario S07. Mutation-verified.
- [x] FIX: `extract list` could only read a field's TEXT, so an ATTRIBUTE-valued field came back empty (a task's due date, a row's href, a status flag). Field selectors now support `@attr` (the item's own attribute), `@sel@attr` (a descendant's attribute) and a pure `[data-x]` selector (an attribute value: first descendant match, else the item itself). Tests: src/contentScript/extractData.test.ts + htrcli/internal/commands/extract_test.go + benchmark scenarios S07/S08. Mutation-verified.
- [x] Enhancement: benchmark harness (`htrcli bench run <scenario-file>` — JSON scenario files with raw htrcli argv steps, expectations (exit/stdoutContains/stdoutNotContains/jsonPath), optional steps, requires_auth -> NOT_RUN, `${VAR}` expansion; writes a JSON report with per-step exit code/attempts/wall_ms and per-scenario commands/wall_ms/retries/failure_mode). Tests: bench_harness_test.go (25). Offline smoke scenario: docs/benchmarks/scenarios/s01-offline-smoke.json (PASSES over CDP).

## Review fixes applied (2026-10-21)

Resolved the `/review-changes` findings for this change set. Each fix has a
regression test that failed beforehand (or, where noted, a real-Chromium check).

- `collect --max-items` now caps inside the harvest, not only the scroll loop, and sets `truncated`. Previously returned the whole DOM (10 rows for `--max-items 3`).
- `collect` restores the list's original scroll position in a `finally`. It used to leave the page scrolled to the bottom.
- `collect --ref` mints each ref at HARVEST time via the new `onRow` hook, so a virtualized/recycled row no longer yields a stale or wrong ref.
- `extract table` excludes the header row wherever it lives (a `<thead>` without a `<tbody>` used to emit it as data) and uniquifies duplicate column names instead of dropping a value.
- `extract list` no longer infers container-vs-item from the match COUNT. Several matches are items; one match is an item unless it is a semantic list/table container; `--container` forces child expansion. A single `.row` used to explode into its child spans.
- `extract list` names the offending field when a field selector is invalid instead of aborting with a bare `DOMException`.
- `fill` on a contenteditable uses a Range scoped to the element instead of `execCommand("selectAll")`, which selected the whole editing host and destroyed a reply box's quoted thread and signature.
- `check`/`uncheck` handle the ARIA tri-state: `aria-checked="mixed"` is neither checked nor unchecked, so it no longer short-circuits into a silent no-op that reported success.
- `prepareKeys` places the caret at the END of a contenteditable, and `handleType`'s new contenteditable branch does the same before `execCommand`. VERIFIED with the real extension loaded (see the F3 entry below): the `handleType` caret call is what stops text landing before the quoted thread. The content script takes the SYNTHETIC `handleType` path for `type`, not the CDP relay.
- `attachShared` coalesces concurrent attaches into one real attach and rolls back a failed one, so a failed attach cannot leave a hold behind for a caller's compensating `detachShared` to release.
- `htrcli bench run --json` exits non-zero for a failing suite (it returned nil before the failure check).
- `htrcli bench` reports the attempts the `retry` wrapper ACTUALLY made (new `retry: attempts=N` stderr line) instead of the granted retry budget, so `commands`/`retries` match the spec.
- `htrcli retry` forwards the persistent flags the user set before `retry` (transport/json/tab/…), which were previously dropped from the child argv.
- `htrcli collect` prints JSON on the CDP path like every sibling command (it printed Go map syntax).
- `htrcli bench run --help` and the `bench.go` package doc show the `{"scenarios":[…]}` wrapper the loader requires.
- `bun run check` (biome) is green again: `htrcli/internal/cdp/formkit.js` carried 5 pre-existing `lint/style/useTemplate` errors (hand-written concatenation in an embedded object literal, not vendored code). Rewrote all 5 to template literals after confirming every operand is a string (`describe()` returns `string` on all paths), so semantics are unchanged.
- Removed `findScrollContainer`'s parent-ancestor fallback: it was unreachable (the first walk already covered every ancestor it could reach except the 13th), so the "Gmail table wrapper" comment was misleading. Replaced by one bounded walk.

### Still unverified after the review fixes (manual steps)

- [ ] **F3 — verified on a Gmail-SHAPED fixture with the real extension loaded; real Gmail still untested.** Harness: Playwright `launch_persistent_context(headless=False, --load-extension=build/)`, driven from the extension service worker with `chrome.tabs.sendMessage({type:'EXECUTE_COMMAND', command:{action:'type', target:{selector:'#composer'}}})` (htrcli / native messaging / bearer auth bypassed; the content-script path is the same one the background uses). Two fixture shapes, both with a quote + signature: minified (no inter-element whitespace) and pretty-printed. RESULT: with the fix, typed text lands AFTER both the quote and the signature in both shapes (i_quote < i_reply, i_sign < i_reply). CONTROL: removing the `handleType` caret call makes BOTH shapes FAIL (text at index 0 / 6, before the quote). So the test discriminates and the fix is causal. PATH NOTE: all four runs emitted `change`+`blur`, which only `handleType` dispatches — the content script uses the SYNTHETIC path for `type`, not the CDP relay (consistent with `chrome.debugger` not being exposed to content scripts). The `prepareKeys` caret call is therefore not exercised by `type` on Chrome. REMAINDER of the step: run it against a real Gmail reply box (selection/focus behaviour in Gmail's own composer can differ).
- [ ] **F11 — `collect --ref` on a live recycling virtualizer.** The `onRow` hook is unit-tested to fire while the row is attached, but a real react-window/react-virtualized list has not been driven. Step: run `htrcli collect "<row selector>" --ref` against a real virtualized list, then use one minted `@eN` in a follow-up command and confirm it resolves to the right row.
- [x] **Benchmark metrics with the corrected attempt counting.** `htrcli bench` now reports observed attempts instead of the granted retry budget. Ran `make bench-smoke` + `make bench-office` against a real Chrome on htrcli's dedicated profile (`~/.htrcli/chrome-profile`, port 9555): all 8 scenarios (S01–S03, S04–S08) are first-try passes — `commands` equals step count and `retries` is 0 on every one (S01=7/0, S02=5/0, S03=5/0, S04=7/0, S05=5/0, S07=9/0, S08=11/0). Reports land in `docs/benchmarks/results/` (gitignored).
- [x] **Bug fix: `extract table` dropped the first row of `<td>`-only grids.** `extractTable()` used to treat the first row as an implied header whenever there was no `<thead>`, silently losing that row's data and turning its first cell into a column name. Benchmark scenario S06 (Sheets fixture, `extract table "#grid"`, expected `rowCount: 3`) failed at 2. Fix: a `<td>` row is always DATA; only a `<thead>` row or an all-`<th>` first row is a header row. No-header tables name columns `col1`, `col2`, … from the widest data row. Tests: `extractData.test.ts` (new td-only grid test that pins S06's rowCount=3, a th-row header test, empty-table tests; changed the old "first tr is header" test). CDP bundle regenerated via `bun run cdp-bundle:build`. S06 now PASSES; S01–S05/S07/S08 stay green.
- [ ] **Real-browser regression for the remaining fixes.** F1/F10/F13 were verified in real Chromium against the shipped bundle; F4/F5/F14/F7/F8/F9/F12/F15 are verified by unit test and the built binary only. No known gap, listed for completeness.
