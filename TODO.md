# TODO

## htrcli network capture — deferred / known limitations

- Response bodies are captured on Chrome (CDP getResponseBody, 64KB cap) but NOT on Firefox (webRequest cannot cheaply read bodies). Firefox network entries are metadata-only.
- The shared debuggerManager is used only by network + dialog capture. The existing trusted-input (cdpInput.ts) and CDP_EVAL paths still do their own chrome.debugger.attach, so running a click/eval command while a capture window is open on the same tab fails with "Already attached". Route those through debuggerManager in a later pass.
- Network mocking/interception (spec §1b) is a separate deferred phase (needs webRequestBlocking + Fetch.enable).

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
  CDP `@eN` drag endpoints resolve through the persistent backend-node ref store.
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
- **The `--browser` flag is persistent and therefore inert for tab-routed
  commands.** `htrcli click --browser firefox` is accepted and ignored, because
  a tab-routed command already knows its browser. Worth rejecting or scoping if
  it starts confusing users.
