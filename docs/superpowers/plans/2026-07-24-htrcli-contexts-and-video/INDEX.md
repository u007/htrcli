# htrcli Browser Contexts + Video/Trace Recording — Implementation Plan Index

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement these plans task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship feature #7 of the htrcli Feature-Parity spec (`SPEC_HTRCLI.md` §7): named browser contexts (`--context`), Chrome video recording (`record start`/`record stop`), and a `trace export` bundle — the spec's highest-risk section, built last.

**Why this is split into parts:** This feature spans three genuinely independent subsystems (process/profile isolation, a CDP screencast → ffmpeg pipeline, and a read-only zip aggregator). Each produces working, testable software on its own and can be reviewed/merged independently. Per the repo's planning rule (split plans > ~300 lines / > 8 tasks), each subsystem is a self-contained part file with no cross-part code references.

## Architecture (whole feature)

```
                 htrcli --context work open ...
                          │
        ┌─────────────────┴──────────────────┐
        │  Part 1: Contexts                   │
        │  contexts.json registry             │
        │  → separate Chrome process/profile  │
        │    on its own --remote-debugging-port│
        └─────────────────┬──────────────────┘
                          │ (a context = a CDP port)
        ┌─────────────────┴──────────────────┐
        │  Part 2: Video (Chrome/CDP only)    │
        │  record start → detached recorder   │
        │    process holds a CDP session,     │
        │    Page.startScreencast → JPEG dir  │
        │  record stop → ffmpeg → out.mp4     │
        └─────────────────────────────────────┘
        ┌─────────────────────────────────────┐
        │  Part 3: Trace export (read-only)   │
        │  GET /api/events (console+network)  │
        │  + screenshot + page info → zip     │
        └─────────────────────────────────────┘
```

## Key design decisions (and honest deferrals)

1. **Contexts = separate process + profile dir, NOT `Target.createBrowserContext`.**
   `internal/cdp/session.go` opens a fresh CDP WebSocket per command and closes it
   immediately (`defer s.Close()`). A context created via
   `Target.createBrowserContext` is destroyed when the browser-level session that
   created it disconnects — incompatible with htrcli's stateless one-shot session
   model without introducing a long-lived context-owning daemon. A per-context
   `--user-data-dir` profile launched as its own Chrome process (the pattern
   `internal/cdp/launch.go` **already** uses for the single default profile) gives
   true OS-level cookie/storage isolation, survives across CLI invocations, and is
   symmetric with the Firefox `-profile <dir>` fallback. **Recommended and chosen.**

2. **Video is Chrome/CDP-only (`internal/cdp/screencast.go`), captured directly by
   a detached Go recorder process — not the extension.** The spec's §7b New-files
   list names `internal/cdp/screencast.go` (Go CDP side). Capturing over the CDP
   socket means Go already holds the JPEG frames in-process and writes them
   straight to a temp dir — there is no native-messaging 1 MB relay hop in the CDP
   path, so the screenshot-style HTTP POST-back is unnecessary here. The recorder
   is a detached process managed exactly like `StartBrowser`/`StopBrowser` manage
   the Chrome PID (state file + SIGTERM). **Deferred:** an extension-transport
   variant (extension attaches `chrome.debugger`, POSTs frames to a new
   `/api/record/frames` endpoint mirroring the screenshot POST-back) — noted in
   Part 2 as the future non-CDP path, not built now.

3. **ffmpeg is a new external prerequisite.** It is a system binary, not a pinnable
   package, so we cannot pin an exact build the way the repo pins npm/Docker
   versions. Instead we detect it at **both** `record start` (fail fast) and
   `record stop`, capture and log its reported version, and document a minimum
   major version (ffmpeg ≥ 6) in the README. A missing ffmpeg always produces an
   explicit error, never a hang.

4. **Firefox video is infeasible, not a fallback.** `record start` under the
   Firefox/extension transport returns a clear "not supported on Firefox" error.

5. **Trace export depends on sibling plans not yet landed.** It aggregates
   `api.EventEntry`/`EventsResponse` (already in `internal/api/types.go`) filtered
   by `kind` (`console` exists today; `network` lands with
   `2026-07-24-htrcli-network-capture.md`) plus a single snapshot screenshot via
   the existing `GetScreenshot()`. **Deferred / revisit when siblings land:**
   per-step full-page screenshots (§3 `2026-07-24-htrcli-fullpage-annotated-screenshots.md`)
   and a timestamped **action** log — no action buffer exists in the daemon today,
   so trace export bundles events + one screenshot + page info only, and Part 3
   flags exactly where the action stream slots in later.

## Execution order

Parts are independent but the recommended order matches the spec's risk ramp:

1. **Part 1 — Browser contexts** (`01-browser-contexts.md`) — 4 tasks. Lowest risk;
   generalizes existing launch code. No new external dependency.
2. **Part 2 — Video recording** (`02-video-recording.md`) — 6 tasks. Highest risk;
   new ffmpeg dependency + detached recorder process + CDP screencast loop.
   Depends on Part 1 only for the optional `--context` port resolution (degrades to
   the default port if Part 1 is absent).
3. **Part 3 — Trace export** (`03-trace-export.md`) — 3 tasks. Read-only aggregator.
   Independently mergeable; produces a partial-but-honest bundle until the network
   and screenshot sibling plans land.

## Global Constraints (apply to every task in every part)

- Go module root: `htrcli/`. Run Go tests with `cd htrcli && go test ./...`.
- Go tests use `net/http/httptest.NewServer` + the `api.ApiResponse{OK,Data,Error}`
  envelope, matching `internal/api/client_test.go` and `internal/commands/*_test.go`.
- CLI verbs guard unsupported transports explicitly: reuse `errUnsupportedCDP(name)`
  (already in `internal/commands/cdp_exec.go`) and add the symmetric Firefox/ext
  guard where a verb is CDP-only — never a silent no-op.
- Never add `--remote-debugging-address`; the debugging port must stay bound to
  localhost (existing rule in `internal/cdp/launch.go`).
- Detached child processes use `cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}`
  and are reaped with a `cmd.Wait()` goroutine, matching `StartBrowser`.
- State files live under `~/.htrcli/`, mode `0600` for files, `0700` for dirs,
  written via `json.MarshalIndent(..., "", "  ")`, matching `writeState`.
- Every caught error is logged with what was attempted + the error, or carries an
  explicit `// intentionally not logged: <reason>` comment (repo rule).
- Listings (`context list`) are sorted by a meaningful key (context name).
- No `latest`/unpinned dependency tags. ffmpeg is documented with a minimum major
  version (≥ 6), not silently assumed.
- Biome for any TS touched (tabs, double quotes) — `bun run check:fix`. Extension
  log prefix `[HTR NControl]`. (Only Part 2's deferred note touches TS; no TS is
  built in this plan.)

## Files created/modified across all parts

**Created:**
- `htrcli/internal/cdp/context.go` + `context_test.go` (Part 1)
- `htrcli/internal/commands/context.go` + `context_test.go` (Part 1)
- `htrcli/internal/cdp/screencast.go` + `screencast_test.go` (Part 2)
- `htrcli/internal/media/ffmpeg.go` + `ffmpeg_test.go` (Part 2)
- `htrcli/internal/commands/record.go` + `record_test.go` (Part 2)
- `htrcli/internal/commands/trace.go` + `trace_test.go` (Part 3)

**Modified:**
- `htrcli/internal/cdp/launch.go` (Part 1 — extract profile-parameterized launch)
- `htrcli/internal/commands/root.go` (Part 1 — `--context` flag + context-aware port)
- `README.md` / `CLAUDE.md` (Part 2 — ffmpeg prerequisite doc)
