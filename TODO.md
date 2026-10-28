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
- [x] B07 Booking — P1. OFFLINE PROXY verified via S10 (docs/benchmarks/fixtures/booking.html + docs/benchmarks/scenarios/s10-booking-fixture.json, run with `make bench-booking`): room select via native <select>, date via popover day grid, time select, seat map with one taken seat, and confirmation number capture (BK-100001). Live booking-site run still needs auth (see BLOCKED line).
- [x] B08 Forward/label/archive email — P1. OFFLINE PROXY verified via S13 (docs/benchmarks/fixtures/mail.html + docs/benchmarks/scenarios/s13-mail-actions.json, run with `make bench-mail`): forward to dave@example.com with a typed note (sent log records recipient + exact note), custom label popup via `check` on the Finance checkbox (row 2 data-labels=Finance), archive proven with `wait --gone` (row 3 absent from the DOM, archive counter 1, 2 rows remain, row 1 untouched). Live Gmail run still needs auth (see BLOCKED line).
- [x] B09 Expense report/invoice — P2. OFFLINE PROXY verified via S11 (docs/benchmarks/fixtures/expense.html + receipt.txt + docs/benchmarks/scenarios/s11-expense-fixture.json, run with `make bench-expense`): dynamic add-line (2 rows), native <select> per row, receipt upload via `htrcli upload` (receipt.txt, 62 bytes, name+size read back), running total 168.25, confirmation reference EXR-700001. Live expense-portal run still needs auth.
- [x] B10 File management — P2. OFFLINE PROXY verified via S12 (docs/benchmarks/fixtures/files.html + report.txt + docs/benchmarks/scenarios/s12-files-fixture.json, run with `make bench-files`): `htrcli upload` on a real file input (report.txt, 72 bytes, name+real size read back), click-based move via per-row native <select> (avoids unsupported HTML5 drag-and-drop), DOM share dialog, share link read back via `value` (https://files.example/s/ABC123), edit permission granted, and an untouched archive file confirmed unchanged. Live Drive/OneDrive/Dropbox run still needs auth (see BLOCKED line).
- [x] B11 Contact/CRM update — P2. OFFLINE PROXY verified via S14 (docs/benchmarks/fixtures/crm.html + docs/benchmarks/scenarios/s14-crm-fixture.json, run with `make bench-crm`): live search filter (non-matching rows removed from the DOM, proven with `wait --gone`, surviving count 1), clear the filter with `clear`, update phone + note, commit two tag chips with `fill` + `press Enter` (input clears after each commit, chips carry `data-tag`), save (row `data-phone`/`data-tags`), REOPEN the record to prove persistence comes from the data model rather than the form, log a call attributed to the edited contact, and confirm a third contact was untouched. Live CRM run still needs auth (see BLOCKED line).
- [x] B12 Messaging — P2. OFFLINE PROXY verified via S15 (docs/benchmarks/fixtures/messaging.html + docs/benchmarks/scenarios/s15-messaging-fixture.json, run with `make bench-messaging`): react ✅ on a message that already carries two 👍 from others (count>0 ≠ reacted-by-me), send from the contenteditable composer with `press Enter` (fill leaves focus there), prove the ✅ state survives the re-render that sending triggers, live search filter (`wait --gone` + `clear`), DM post via the Send button, and channel isolation proven in both directions. Thread replies are NOT covered (distinct interaction, noted in the spec's gaps). Live Slack/Teams run still needs auth (see BLOCKED line).

- [x] B12a Thread replies (messaging) — P2. OFFLINE PROXY verified via S17 (docs/benchmarks/fixtures/messaging.html + docs/benchmarks/scenarios/s17-thread-replies.json, run with `make bench-messaging`): the fixture gained a per-message Reply control and a thread panel with its own contenteditable, and replies live in a `threads` map OUTSIDE the channel array so a mis-commit cannot pass by accident. Two parents start at different counts (m1=0, m2=2) so a wrong-parent reply is a numeric mismatch rather than a vague one. Proven: a reply typed into `#thread-input` and sent with Enter raises m2 to 3 while `#messages` stays at 3 and `stdoutNotContains` the reply text; the count survives a switch to another channel and back (model-backed); a reply to m1 gives m1=1 with m2 still 3; closing sets `#thread-panel[hidden]`; `#composer` stays empty (the channel composer was never the target). No htrcli command was missing — `click` + `fill` + `press Enter` covered it. 5 mutants caught (reply into the channel, hardcoded parent, threads cleared on channel switch, hidden never set, count capped at 2). Live Slack/Teams run still needs auth (see BLOCKED line).
- [x] B13 Form filling — P2. OFFLINE PROXY verified via S09 (docs/benchmarks/fixtures/form.html + docs/benchmarks/scenarios/s09-form-fixture.json, run with `make bench-form`): multi-page form with required-field validation, native <select>, date fill, country pick, confirmation reference capture (EXP-100001), and no error showing after correct fill. Live form run still needs an actual expense portal.
- [x] B14 Research & summarize — P2. OFFLINE PROXY verified via S16 (docs/benchmarks/fixtures/research-source-{a,b}.html + research-summary.html + docs/benchmarks/scenarios/s16-research-fixture.json, run with `make bench-research`): captured a Q4 figure AND an opaque reference from each of two separate pages into `${a_revenue}`/`${a_ref}`/`${b_churn}`/`${b_ref}`, composed the summary from those four (asserting no literal `${` survived — `stdoutNotContains`), rejected an empty summary and then a written summary with no source, cited both pages, saved, and read back the exact sentence plus a `stdoutNotContains` guard proving the Q3 column was not mistaken for Q4. "No hallucinated facts" is NOT provable offline — this proves the read→carry→cite→save workflow only; the live research run still needs target sites that permit reads (see BLOCKED line).
- [x] B17 Support ticket create/update — P3. OFFLINE PROXY verified via S18 (docs/benchmarks/fixtures/ticket.html + docs/benchmarks/scenarios/s18-ticket-fixture.json, run with `make bench-tickets`): two seeded tickets carry different statuses and assignees (TKT-1040 Open/unassigned, TKT-1041 Waiting/alice) so a cross-ticket write is visible. Create is rejected twice for missing subject then missing description, then submitted with subject/description/category/priority/attachment (report.txt). The generated id is CAPTURED and reused as a selector (`[data-ticket-id='${ticket_id}']`) — the first scenario that needs captures inside expectations, which the harness now supports. Update half: Resolve-without-assignee is refused, the rejection is proven not to persist by going back to the list (a model re-render), status+assignee are then saved and read back from the row, a comment is posted and the row's count moves 0 -> 1, and both seeded tickets are proven untouched. 5 mutants caught (comment on the wrong ticket, status not persisted, subject validation removed, confirmation id differing from the row id, rejected save persisting). Live helpdesk run still needs auth (see BLOCKED line).
- [x] B19 HR leave request — P3. OFFLINE PROXY verified via S19 (docs/benchmarks/fixtures/leave.html + docs/benchmarks/scenarios/s19-leave-fixture.json, run with `make bench-leave`): 73 commands. This is the first scenario whose correct answer is a number the page computes, so the count is pinned three ways — a Monday→Sunday week must be 7 (an off-by-one gives 6, "business days only" gives 5; both mutants were caught on the same assertion). Balance is read-modify-written in both directions (14 → 7 on a 7-day request, back to 14 on cancellation) and proven across a reload, which required implementing `reload` over `--cdp` first. Also: half-filled range shows no duration, single day reads "1 day", inverted range shows nothing live AND is refused on submit, missing type / missing reason each named separately, a 30-day request refused against a 14-day balance with no row and no confirmation, and the pre-existing approved request untouched. A fresh run clears the fixture's storage first — the scenario proves persistence, so it must not inherit the previous run's requests. 6 mutants caught (exclusive count, business-days-only count, balance not decremented, cancel not restoring days, over-balance allowed, nothing persisted). Live HR portal run still needs auth (see BLOCKED line).
- [x] B15 Social post + schedule — P3. OFFLINE PROXY verified via S20 (docs/benchmarks/fixtures/social.html + docs/benchmarks/scenarios/s20-social-fixture.json, run with `make bench-social`): 88 commands. The character limit is pinned by a SUBMIT, not the counter — a counter-only scenario let a `>=` off-by-one in the validator survive (real mutant, caught by mutation testing before this landed). Boundaries proven: 280 accepted and stored untruncated, 281 and 300 refused with both numbers quoted, empty composer refused from BOTH buttons, missing time / past time / exactly-the-clock-time refused. Time is deterministic because the fixture carries a fixed demo clock (`data-now`), so past/now/future never depend on the machine's date. Scheduled and published are distinct states (`data-state` + differing label/when), both survive a `reload` (the feature built for S19), and editing a scheduled post RESUMES it in the composer and reschedules IN PLACE — proven not to duplicate by count-before/after. 7 mutants caught (cap `>=`, cap removed, `<=`→`<` boundary, future check removed, edit duplicates, nothing persisted, counter not live). Live social platform run still needs auth (see BLOCKED line).
- [x] B20 Survey / feedback — P3. OFFLINE PROXY verified via S21 (docs/benchmarks/fixtures/survey.html + docs/benchmarks/scenarios/s21-survey-fixture.json, run with `make bench-survey`): 77 commands. The first task whose artifact is a set of answers rather than one record. Proven: Next-with-nothing names only the FIRST missing question and advances to the next one once it is answered (a single generic gate would repeat the first); a Back/forward round trip restores the radio (`:checked`) and the select (`--json value`) from the page's model; the matrix records each ROW separately (answering Tooling leaves "Please rate: Onboarding"); the multi-select gate only appears once the matrix is complete, and both checkboxes are recorded (`Excel, Slack`) rather than just the last clicked one; edits made from the review page — a checkbox removal and a matrix change — are reflected without disturbing neighbouring answers; the 10-character minimum is pinned at the boundary (9 refused, 10 accepted); the captured id appears in the response list, a second submit is refused with the row count unchanged, and after `reload` the same id is shown with `body[data-submitted='1']` while the seeded SUR-5001 still reads Hybrid. **One mutant initially SURVIVED**: removing the page-1 restore did not fail, because a hidden step keeps its DOM values — `:checked` passed for the wrong reason. Fixed the FIXTURE (clear every input before each render, like a framework remount) so the restore is load-bearing; 8 mutants then caught (radio not recorded, all matrix rows written to one key, restore removed, page-1 gate removed, `<=` boundary, no persistence, duplicate-submit guard removed, tools not accumulated). htrcli needed no new command: `check` drives radios through the native `checked` setter, `wait` matches `:checked`/attribute selectors, `select` takes the option VALUE. Live survey run still needs auth (see BLOCKED line).
- [x] B18 Bank / finance read-only check — P3. OFFLINE PROXY verified via S22 (docs/benchmarks/fixtures/finance.html + docs/benchmarks/scenarios/s22-finance-fixture.json, run with `make bench-finance`): 62 commands. The first scenario whose job is to prove a NEGATIVE, and the first with a literal `$` in its expected data. Proven: the current balance reads `$4,820.50` (and `4820.50` through `attr data-amount`, captured and reused in later expectations) while AVAILABLE is a deliberately different `$4,760.50`, so the wrong card cannot pass; six rows in the default period with the 28 March row provably absent; a wider filter brings it back and moves the period total +$520.50 → -$117.50 → +$460.50 without ever changing the balance; `extract table` reads headers + rows; the LAST running balance equals the header balance (reconciliation); a second account reads `$12,483.07` and its predecessor's rows are gone (reads are account-scoped, not sticky); and then the read-only half — the access log records every view/filter, so the log is demonstrably LIVE and `tr[data-action='transfer']` is proven able to match, `body[data-writes]` stays 0, the transfer panel is opened but never submitted, and a `reload` leaves balance, default period, log count and zero writes intact. **This scenario found a real htrcli bug**: `os.Expand` treated the `$4` in `"$4,760.50"` as a variable named `4`, rewriting the expectation to `"${4},760.50"` once any capture existed (the earlier steps passed only because the capture map was still empty). Fixed with `expandBenchRef` (`bench_run.go`): a reference must be a shell-valid identifier (`${name}` / `$name`), every other `$` stays literal, and an unknown name keeps its original form. Tests: `TestExpandBenchRefKeepsCurrencyLiteral` + `TestSubstituteExpectCapturesLeavesCurrencyAlone`. 8 mutants caught (os.Expand semantics restored, filter ignored, thousands separator dropped, wrong account, available showing the current balance, Apply logging a transfer, period total ignoring the filter, reads not logged); the "Apply logs a transfer" mutant also PROVED the `--gone` transfer selector matches when such an entry exists, so the negative assertion is not vacuous. Live bank run still needs auth (read-only, staging only).
- [x] B16 E-commerce reorder — P3. OFFLINE PROXY verified via S23 (docs/benchmarks/fixtures/store.html + docs/benchmarks/scenarios/s23-store-fixture.json, run with `make bench-store`): 95 commands. The first task whose artifact is DERIVED from history, so the benchmark has to tell "re-ordered" from "copied the old order". Three state facts decide it and all three are asserted: a NEW order id (ORD-2031, captured from the confirmation and reused in later selectors), exactly one additional row in the history, and the source order unchanged ($42.95, still Delivered) — plus a cleared cart, so "items re-added but never checked out" is distinguishable. The core trap is that quantities and prices travel differently: quantities carry over from ORD-1042 (notebook 2, pen pack 1, read back with `--json value`) while prices come from today's catalogue, and the pen pack is $7.25 today against $6.00 paid — a fixture whose price never moved could not tell a re-order from a copy. The discontinued Desk Organiser is the third case: still in the cart table (rowCount 3) marked unavailable with the reason on the page, no quantity control, excluded from the subtotal ($24.25 not $39.25), and absent from the review page (rowCount 2). Shipping is its own line so subtotal + $4.95 = $29.20, and the commit button states the grand total. A quantity edit 1→2 recomputes to $31.50/$36.45 and back (proving totals track the model). Checkout gates ONE field at a time — address, then payment, each error naming itself — then the review shows the office address and `•••• 4242`. Safety is asserted, not assumed: the adjacent "Buy again (1-click)" decoy is PROVEN reachable (`wait` on the button) yet the audit log has zero `one_click` entries and `data-one-clicks` stays 0, and the new-card form is never opened (`data-card-entries` 0). Then a two-step cancel: first click only opens the confirmation (status still Processing), "Keep order" dismisses it and leaves Processing, confirming sets Cancelled, removes the control, keeps the row (status change, not delete), and survives a `reload`. 13 mutants caught — qty reset to 1, price never changed, discontinued counted as available, total = subtotal, review keeps the unavailable line, validation order swapped, cancel without confirmation, cancel on every order, cancelled order keeps its button, placed total = subtotal, qty change not recomputing, reorder mutating the source order, and reorder REUSING the source order id. htrcli needed no new command (`check` for radios, `--json value` for quantities, `--json attr` for status/write counters, `extract table|list` for counts). Live store run stays BLOCKED (test store, cancel if possible, never a real card).
- [x] Enhancement: DOM-level `wait` command (selector/text/URL) both transports (htrcli wait).
- [x] Enhancement: rich-text (contenteditable) typing helpers (handleFill/handleType use document.execCommand selectAll/delete/insertText + input/change/blur dispatch).
- [x] Enhancement: virtualized-list extraction helper (`htrcli collect` — scrolls the list container, accumulates + de-dupes rows; both transports).
- [x] Enhancement: retry-with-backoff wrapper (`htrcli retry -- <cmd>` with `--times`/`--delay`/`--max-delay`; opt-in, logs every failed attempt to stderr, non-zero exit on exhaustion; works on both transports since it re-invokes htrcli; a transport flag placed before `retry` is forwarded too). Tests: retry_test.go (5).
- [x] Enhancement: `wait --gone --selector` — waits until NO element matches, the only way to assert an archive/delete/move actually removed a row. "Gone" means absent from the DOM, so a hide-only UI times out (intentional). Both transports; `--gone` requires `--selector` and rejects `--text`/`--url`. Tests: commandExecutor.test.ts (6) + bench_test.go flag matrix and gone-mode helpers; scenario S13.
- [x] Enhancement: `reload` over `--cdp` (`cdp.Reload`) — it used to answer "reload is not supported over --cdp yet", so NO scenario could prove anything survived a page reload, which is how persistence has to be shown. Now Page.reload + wait for `Page.loadEventFired` (not a re-navigate: a repeat navigate of the same URL can be a same-document navigation that keeps the old JS state alive and makes the assertion vacuous). Tests: internal/cdp/nav_reload_test.go — verb pin + a timeout test proving it actually waits; both mutants caught (navigate instead of reload; dropping the wait). First user: S19.
- [x] GAP (documented, not fixed): `--json attr` DROPS an empty attribute value — `api.CommandResult.Data` is `json:"data,omitempty"` (htrcli/internal/api/types.go:52), so asserting "this attribute is the empty string" is unwritable (the response is just `{"success":true}`). Same root cause as the `fill ""` gap above (request side). Workaround used in S19: assert visibility semantically (`wait --selector "#confirmation.visible" --gone`) and pair it with a positive `wait` so the selector is proven able to match. A wire-level fix means adding a "present" flag or always emitting `data` — deliberately not done mid-benchmark because it changes every command's response shape. Hit again in S21: "the error area is empty after a successful submit" is equally unwritable, so the submit is proven by the confirmation banner and the row count instead.
- [x] Enhancement: captures now expand inside expectation strings (`stdoutContains`, `stdoutNotContains`, `jsonPath.equals`), not only in argv — without it a scenario could target a captured id in a selector but could not assert that the record opened on it. Captures shadow env vars there too; an unresolved name stays literal and fails. Tests: 3 in bench_capture_test.go (`expectRunner` evaluates expectations like the real runner — `captureRunner` reports OK unconditionally and made the first draft of these tests vacuous); mutant = removing the `prepared.Expect = substituteExpectCaptures(...)` wiring fails 2 of them. Scenario: S18.
- [x] GAP (documented, not fixed): `extract list` FAILS with `Element not found` when the selector matches nothing, so "zero rows" cannot be asserted with it. Deliberate — a typo'd selector must not read as "no data". Assert emptiness with `wait --selector "<container>:empty"` (used by S17).
- [x] Enhancement: step-output capture + `${var}` substitution (`"capture": {"name": "items.0.q4"}`) and `${VAR}` expansion in expectation strings. Captures are per-scenario, taken only from a PASSED step, never re-scanned, and a missing path fails the step. Tests: bench_capture_test.go (9) + bench_expect_vars_test.go (3); 3 mutants each caught (in-session: substitution removed, wrong column captured, citation gate removed). Scenario: S16.
- [x] GAP (documented, not fixed): `fill` cannot set an EMPTY value — the empty string is dropped from the command payload (`Value string \`json:"value,omitempty"\``), so `fill "#q" ""` fails with `requires a "value"`. Use `clear` (it dispatches input/change too). Found while building S14; documented in skills/htrcli/SKILL.md → Interaction commands. Rejected a wire-format change (pointer/extra flag) as too invasive for a case `clear` already covers.
- [x] Enhancement: structured data extraction command (`htrcli extract table|list`; TS handlers + Go command + tests + bundle).
  - Chosen over retry-with-backoff because B01 acceptance (structured list with sender/subject/date) needs extraction first; collect returns raw element info, extract turns it into records. (Retry/backoff has since been added too — see the entry above.)
- [x] LIVE B01 run (2026-10-27, read-only, extension transport, tab 1100667979): **PARTIAL**. Search works — `fill "input[name=q]" "is:unread"` + `press Enter` reached `#search/is%3Aunread` and Gmail reported "1–37 of 37" — and `extract list "tr.zA" --fields "sender:span.yP,subject:span.bog,date:@td.xW span@title"` returns structured records. Two defects fail the "no wrong-folder rows" criterion; both were tracked below and are now FIXED in code (see the `--visible` and `||` entries). The mailbox was left unchanged: no send/delete/archive/mark/draft, view restored to `#inbox` (verified: hash `#inbox`, 50 visible rows, 50 unique threads, 0 duplicates). Detail recorded in `docs/benchmarks/office-task-benchmark.md` (B01 "Live run").
- [x] Enhancement: `extract list` VISIBILITY filter (`--visible`). Live Gmail keeps the previous view in the DOM: `tr.zA` matched 87 rows for 37 conversations — 37 visible (correct) + 50 hidden, including 36 READ rows in an `is:unread` search and 14 duplicate unread rows (73 unique thread ids). `--visible` drops items with no layout box (`checkVisibility({checkOpacity,checkVisibilityCSS})`, else `getClientRects().length === 0`; with neither API it ERRORS rather than silently including hidden rows (fail loud, not open)). OPT-IN on purpose — a host without a layout engine reports zero rects for everything, so default-on would empty every jsdom/bun result. Implemented in `src/contentScript/extractData.ts` (`ExtractListOptions.visibleOnly`, `isRendered`) + `commandExecutor.ts` + the Go flag; the checked-in CDP bundle was regenerated. Verified end-to-end over CDP with a probe page (3 rows, one inside `display:none`): 3 without the flag, 2 with it. Tests: 4 in extractData.test.ts + a Go passthrough test; mutants caught: always-visible, flag not forwarded.
- [x] Enhancement: `collect` VISIBILITY filter (`--visible`) — the remaining half. An SPA keeps its previous view mounted, so a collection could accumulate rows the user cannot see, which is worse than in `extract list` because collection runs across many harvests. Implemented in `src/contentScript/collectList.ts` (`CollectOptions.visibleOnly`, filter at harvest time BEFORE the item cap and before `seen`) + `commandExecutor.ts` + the Go flag; the CDP bundle was regenerated. The ordering is load-bearing and pinned: a hidden row is not marked seen, so a row that a later scroll renders is still collected exactly once — reverting to a once-per-element decision (a `hiddenOnce` set) fails that test while the drop test still passes. Verified end-to-end over CDP on a 14-row probe page with two `display:none` rows: 14 without the flag, 12 with it; `#static .row` → 3. Tests: 4 in collectList.test.ts + 2 Go (`TestCollectPassesVisibleThrough`, `TestCollectOmitsVisibleUnlessAsked`).
- [x] Enhancement: `--fields` ALTERNATIVE selectors (`sender:span.yP||span.zF`), first NON-EMPTY value wins. Live Gmail varies markup by row state — sender `span.yP`/`span.zF`, subject `span.bog` (read)/`span.bqe` (unread), date `td.xW span[title]`/`span.bq3` — so one selector per field yielded only 36/50 complete rows. `||` (not `|`) is the separator because `|` is live CSS (`[lang|=en]`); the splitter is quote-aware so `[title="a||b"]` is not torn. Tests: 7 in extractData.test.ts (incl. both splitting traps) + Go passthrough; mutants caught: no fallback, single-pipe split.
- [x] Documentation: `extract --help` now says the `@.link@href` dot is the CSS class marker (form is `@<css>@<attr>`, e.g. `@td.xW span@title`), documents `||` alternatives and `--visible`. Behaviour unchanged: an unmatched but valid selector still returns `""` — whether it should error is still an open question.
- [ ] SKIPPED (user choice, 2026-10-20): B02-B20 live benchmark runs — all need authenticated web sessions (Gmail, Calendar, Docs, Sheets). The reusable pieces they need (`wait`, `collect`, contenteditable typing) are implemented; each scenario's live run is tracked in its own line above.

## Blocked on live auth (user choice — do not run without a confirmed throwaway account)

The following items cannot be verified offline because they require an authenticated session on a real web app. Per the user's standing rule, production data is never touched; if staging is not certain, stop and ask before proceeding. The `gws` CLI session (201 real messages) is treated as production and has not been read, written, or run against.

- [ ] BLOCKED B01 Find emails (Gmail) — needs live Gmail auth. `wait` DONE, `collect` DONE — only the live run remains.
- [ ] BLOCKED B02 Reply to email (Gmail) — needs live Gmail auth. `type` append + signature preservation verified offline via S03.
- [ ] BLOCKED B03 Arrange meeting (Calendar) — needs live Calendar auth. Date/time pickers + guest `pick` verified offline via S04.
- [ ] BLOCKED B05 Google Docs create/edit/share — needs live Docs auth. Contenteditable `type`/`press` verified offline via S05.
- [ ] BLOCKED B06 Sheets/Excel data + formula — needs live Sheets auth. Cell entry + grid read-back verified offline via S06.
- [ ] BLOCKED B08 Forward/label/archive email (Gmail) — needs live Gmail auth.
- [ ] BLOCKED B10 File management (Drive/OneDrive/Dropbox) — needs cloud drive auth.
- [ ] BLOCKED B11 Contact/CRM update — needs a CRM web app.
- [ ] BLOCKED B12/B12a Messaging incl. thread replies (Slack/Teams/Discord) — needs messaging web auth; the offline thread workflow is covered by S17.
- [ ] BLOCKED B14 Research & summarize — needs real sites that permit reads; the offline workflow (capture → compose → cite) is covered by S16.
- [ ] BLOCKED B15 Social media post/schedule — needs social platform auth; the offline compose/schedule/reschedule chain is covered by S20.
- [ ] BLOCKED B16 E-commerce reorder — needs a test store; the offline re-order chain (carried quantities vs today's prices, unavailable line, new order id, 1-click never used, two-step cancel) is covered by S23.
- [ ] BLOCKED B17 Support ticket create/update — needs a helpdesk web app; the offline create→update chain is covered by S18.
- [ ] BLOCKED B18 Bank/finance portal — needs bank web auth (read-only); the offline read-balance/statement/reconciliation chain plus the read-only audit proof is covered by S22.
- [ ] BLOCKED B19 HR leave request — needs an HR portal; the offline request/cancel/persistence chain is covered by S19.
- [ ] BLOCKED B20 Survey/feedback — needs a survey web app; the offline multi-page/required/back-navigation chain is covered by S21.

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
- `collect --ref` mints each ref at HARVEST time via the `onRow` hook while the node is live; once collect scrolls a recycling virtualizer the harvested nodes detach and refs resolve as stale, so callers must use the ref immediately or re-harvest (live check: react-virtualized demo, refs @e1–@e16 resolved via `text`/`attr`).
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
- [x] **F11 — `collect --ref` on a live recycling virtualizer.** Driven against `https://bvaughn.github.io/react-virtualized/` (author react-virtualized demo): `collect "div[role=\"gridcell\"]" --ref` minted @e1–@e16 for all 16 visible rows without scrolling (`steps: 0`); follow-up `text @e1`→"Peter BrimerThis is row 0", `text @e8`→"VVelvet NateraThis is row 7", `attr @e1 role`→"gridcell" all resolved. Contract: refs are valid only at harvest time — once collect scrolls a recycling list, harvested nodes detach and refs report "stale ref". Re-harvest or `find --ref` on the currently-visible row after a scroll.
- [x] **Benchmark metrics with the corrected attempt counting.** `htrcli bench` now reports observed attempts instead of the granted retry budget. Ran `make bench-smoke` + `make bench-office` against a real Chrome on htrcli's dedicated profile (`~/.htrcli/chrome-profile`, port 9555): all 8 scenarios (S01–S03, S04–S08) are first-try passes — `commands` equals step count and `retries` is 0 on every one (S01=7/0, S02=5/0, S03=5/0, S04=7/0, S05=5/0, S07=9/0, S08=11/0). Reports land in `docs/benchmarks/results/` (gitignored).
- [x] **Bug fix: `extract table` dropped the first row of `<td>`-only grids.** `extractTable()` used to treat the first row as an implied header whenever there was no `<thead>`, silently losing that row's data and turning its first cell into a column name. Benchmark scenario S06 (Sheets fixture, `extract table "#grid"`, expected `rowCount: 3`) failed at 2. Fix: a `<td>` row is always DATA; only a `<thead>` row or an all-`<th>` first row is a header row. No-header tables name columns `col1`, `col2`, … from the widest data row. Tests: `extractData.test.ts` (new td-only grid test that pins S06's rowCount=3, a th-row header test, empty-table tests; changed the old "first tr is header" test). CDP bundle regenerated via `bun run cdp-bundle:build`. S06 now PASSES; S01–S05/S07/S08 stay green.
- [ ] **Real-browser regression for the remaining fixes.** F1/F10/F13 were verified in real Chromium against the shipped bundle; F4/F5/F14/F7/F8/F9/F12/F15 are verified by unit test and the built binary only. No known gap, listed for completeness.

## wait --gone — known limitations (2026-10-24)

- CDP `@eN` refs are not translated for `wait`: `waitCDPSelector` sends the ref straight to the page bundle, whose refRegistry only knows refs minted in-page, so `wait @eN` on `--cdp` fails with "not known on this page" on every tick. The detached-element branch (`isDetachedRefError` in `htrcli/internal/commands/wait.go`) is therefore unreachable on CDP until refs are resolved through the CLI ref store (backendNodeId) inside the wait loop. Found by code reading; not run against a browser.
- Extension compatibility is checked by response shape (`wait --gone` requires `data.gone == true`), because the `identify` handshake carries no protocol or capability version. Add a version/capability field so an old extension is refused by name rather than by its answer.
