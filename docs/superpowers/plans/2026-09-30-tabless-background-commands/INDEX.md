# Tab-less Background Commands + `--browser` Hint — Implementation Plan Index

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement these plans task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every `htrcli recordings` command work when no `http/https` tab has a live content script, and let a caller prefer a particular browser via an advisory `--browser` hint.

**Architecture:** Today a nil tabID is resolved by `Daemon.FirstTabID()`, which scans a Go **map** — so it returns 404 with zero tabs and, with several tabs, an arbitrary one. This plan gives each relay connection a monotonic sequence number at accept time and a browser identity announced by the extension, then adds a tab-less `POST /api/background/command` route that selects the lowest-sequence eligible connection. The `--browser` flag is a **hint**: a non-matching hint falls back to first-wins rather than failing.

**Tech Stack:** Go 1.22+ (daemon, api client, cobra CLI), TypeScript + Bun test (extension service worker), Biome (format/lint).

**Spec:** `SPEC_BACKGROUND_COMMANDS.md` (repo root) — the plan argues from it; executors must read both.

> **STATUS (2026-10-01): all three parts are IMPLEMENTED and VERIFIED.** The
> checkboxes below are deliberately left unticked: the work did not land as a
> linear walk through this plan, so ticking them would misreport how it happened.
> What is true today, with the evidence for each claim:
>
> | Part | Implemented in | Pinned by |
> |---|---|---|
> | 1 — daemon identity + deterministic selection | `host/daemon.go` (`seq`, `browser`, `nextConnSeq`, `earliestConnWithTabs`, `lowestTabID`) | `daemon_background_test.go` (8), `daemon_background_internal_test.go` (3), `daemon_determinism_test.go` (1) |
> | 2 — wire + route | `nativeHost.ts` (`identify` send), `host/bridge.go` (inbound `identify`), `host/server.go` (`POST /api/background/command`), `api/client.go` (`ExecuteBackgroundCommandInto`) | `server_background_test.go` (8), `client_background_test.go`, `client_background_alloc_test.go`, `src/background/nativeIdentify.test.ts` |
> | 3 — CLI + docs | `commands/root.go` (`--browser` + `validateBrowserHint`), `commands/recordings.go` (tab-less route), `api/types.go` (`RecordingListData.Browser`) | `browser_flag_test.go`, `recording_list_browser_test.go` |
>
> Verified: `bun run check` / `bun run test` (389 pass, 0 fail) / `bun run build` /
> `bun run firefox:typecheck` / `bun run firefox:build`; `go build` / `go vet` /
> `go test -count=1 ./...` (8/8 packages); `go test -race -count=50 -run FirstTabID`;
> `make htrcli-build`. `gofmt -l` reports only `publish.go`, already unformatted at HEAD.
> Nothing was committed.
>
> **Caveat:** `daemon_determinism_test.go` was accidentally overwritten during
> this work and has been *reconstructed* from the write tool's saved diff — 44 of
> its lines are recovered verbatim, but its final two closing braces were
> inferred from a truncated diff. It is gofmt-clean, passes, fails under the
> `seq`-ignoring mutation, and matches the assertion its `daemon.go:350` comment
> references. Treat it as reconstructed rather than byte-identical to the original.

## Why this is split into parts

The work spans three layers that are independently reviewable and independently testable: daemon state, the wire/route, and the CLI+docs surface. Per the repo planning rule (split above ~300 lines or 8 tasks), each is a self-contained part.

Ordering is strict: **Part 1 → Part 2 → Part 3.** Later parts consume named functions from earlier ones (Part 2 calls Part 1's connection-browser setter and tab-less enqueue; Part 3 uses Part 2's client method), so the sequence is a real dependency, not a preference. No implementation is duplicated across parts, and each part ends at a point where its own test suite is green.



```
  htrcli recordings start|stop|status|list|get|export|delete
                              │
                     ┌────────┴─────────┐
                     │ Part 1 (daemon)  │  RelayConn.seq, RelayConn.browser
                     │                  │  EnqueueBackgroundCommand
                     │                  │  FirstTabID determinism fix
                     └────────┬─────────┘
                              │ (a connection, not a tab, is addressable)
                     ┌────────┴─────────┐
                     │ Part 2 (wire)    │  extension sends `identify`
                     │                  │  bridge stores it
                     │                  │  POST /api/background/command
                     │                  │  client ExecuteBackgroundCommandInto
                     └────────┬─────────┘
                              │
                     ┌────────┴─────────┐
                     │ Part 3 (CLI)     │  --browser hint
                     │                  │  recordings verb rewired
                     │                  │  list shows which browser answered
                     │                  │  6 doc files + full verification
                     └──────────────────┘
```

## The bug this fixes, measured

A throwaway harness registered two connections with three tabs each and called `FirstTabID()` 2000 times against **unchanged** state:

```
DISTINCT_TABS=6 [1 2 3 7 8 9]
COUNTS=map[1:1344 2:208 3:206 7:181 8:34 9:27]
```

All six answers occurred, unevenly distributed. `FirstTabID`'s own comment claims "first match wins"; it cannot honour that, because `d.conns` and `rc.tabs` are both Go maps with randomized iteration order. This is a **pre-existing bug independent of the recording feature** — with Chrome and Firefox both connected, `POST /api/command` with no tab picks a different browser from call to call.

## Global Constraints

- Package manager is **bun only** — never npm/yarn.
- Lint/format is **Biome** (tabs, double quotes); run `bun run check:fix` before finishing any TS task.
- Go style: run `sh "<skill-dir>/scripts/run-tool.sh" list --file-path <file>` before editing Go; the relevant guidelines for this work are `range_over_int`, `min_max`, `cmp_or`, `any`, `strings_cut`, and `slices_contains`.
- Extension error/warn logs use the `[HTR NControl]` prefix.
- New message types go into the `MessageType` union in `src/types/recording.ts` **with a matching interface**.
- Async `chrome.runtime.onMessage` listeners must `return true`.
- `src/` is shared by Chrome and Firefox, so **both must behave identically**; `bun run firefox:typecheck` is a gate on every TS task.
- Any listing (`recordings list`) stays sorted and paginated.
- **The working tree is dirty** (~27 files, all part of the in-progress session-recording feature). Re-read each file immediately before editing. Never `git reset` or `git checkout` them — `git checkout <path>` is denied by `.claude/settings.json`; restore via copying from `git show HEAD:<path>` if ever truly needed.
- **Do not commit unless the user asks.**

## Review Focus

Five input classes the spec implies that individual tests are most likely to miss:

1. **Hint names a browser that is not connected** (e.g. `--browser firefox` with only Chrome up). Expected: falls back and answers from Chrome; `recordings list` then shows `chrome` so the fallback is visible. Never a hard error, never a silent wrong-profile read.
2. **Two connections advertising the same browser.** Expected: lowest sequence wins, deterministically — the same connection on every repeat call.
3. **The winning connection's relay drops mid-session.** Expected: the next-lowest sequence becomes the winner automatically; no command is ever written to the dead connection, and no promotion bookkeeping is needed.
4. **An extension too old to send `identify`.** Expected: its connection records an empty browser, never matches a hint, and first-wins still works. The feature must not hard-require the new message.
5. **A `success:false` result whose `error` is empty.** Expected: a non-empty error naming the failing action. A bare empty string reads like success at the call site (already covered for the tab-routed path; the new path must match).

## Verification gates (all must be shown passing)

Run from the repo root unless noted:

- `bun run check` (Biome)
- `bun run test`
- `bun run build`
- `bun run firefox:typecheck`
- `bun run firefox:build`
- `cd htrcli && go build ./... && go vet ./...`
- `cd htrcli && go test -count=1 ./...`
- `make htrcli-build`

`gofmt -l htrcli/internal htrcli/cmd` must report only `publish.go`, which is already unformatted at HEAD and out of scope.

## Parts

- [Part 1 — Daemon connection identity and deterministic selection](01-daemon-connection-identity.md)
- [Part 2 — Wire protocol and tab-less route](02-wire-and-route.md)
- [Part 3 — CLI surface and documentation](03-cli-and-docs.md)
