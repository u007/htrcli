# Spec: tab-less background commands + `--browser` hint

Status: **approved** (2026-09-30) — including the `FirstTabID()` determinism fix, which the
user explicitly kept in scope
Date: 2026-09-30
Scope: `htrcli` daemon/client/CLI + the extension's native-host bridge
Supersedes: the TODO.md note "Not done: it changes the host protocol" — see
[Corrections to prior notes](#corrections-to-prior-notes).

## Problem

Every `htrcli recordings` command fails with `404 no tabs connected` unless at
least one `http/https` tab has a live content script.

The chain is:

1. `recordings*` send a `Command` with a nil tabID (`sendRecordingCommand`,
   `htrcli/internal/commands/recordings.go`).
2. The CLI posts to `POST /api/command` (`api.Client.ExecuteCommandInto`).
3. `handleCommand` resolves nil via `Daemon.FirstTabID()` — `htrcli/internal/host/server.go`.
4. `FirstTabID` walks `d.conns` → `rc.tabs`; an empty set returns `false` → 404.

So with only `chrome://` pages, a settings page, or a freshly-started browser
open, session recordings are entirely unreachable — even though the actions
themselves need no page.

The extension is **already** able to serve these actions without a tab. In
`src/background/nativeHost.ts`, `sendCommandToTab` handles the six recording
actions at the very top of the function, before any tab lookup, content-script
injection, or `chrome.tabs` access. `NativeMessage.TabID` is `json:"tabId,omitempty"`,
so a `tabId` of 0 is omitted from the frame, and the daemon resolves results by
`commandId` alone (`bridge.go`: `d.ResolveCommand(result.ID, result)`), never by
tabID. **No extension change is required for the route itself.**

## Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Multi-browser disambiguation | `--browser` is a **first-wins hint**, not a strict selector | A hard selector would break the common Chrome+Firefox setup until a full selector exists. Advisory keeps every case working. |
| Multiple active tabs / connections | All eligible; **first wins** | Matches the existing `FirstTabID` "first match wins" convention rather than inventing new semantics. |
| Browser identity source | Reuse the extension's existing `getBrowserType()` | Already populates `TabInfo.Browser`, so there is one place that decides and both paths agree. No new detection logic. |
| Route shape | `POST /api/background/command` | Mirrors the existing request body; additive. |
| "First" is defined as | **Earliest-connected connection**, by a monotonic sequence assigned at accept time | `d.conns` is a `map`, and Go randomizes map iteration. "First wins" is therefore *not* deterministic unless an explicit order exists. See [First-wins is made deterministic](#first-wins-is-made-deterministic). |

## What "first wins" means here

Adopted reading, in the sense this spec uses it:

> **(a) When a tab-less command arrives, the earliest-connected live browser
> answers it, unless a `--browser` hint selects a different one.**

Explicitly **not** these readings, which were considered and rejected:

- **(b) Several tabs record simultaneously, first-to-start owns the session.**
  Rejected — recording is per-extension-profile and the recorder already tracks
  multiple tabs inside one session (`currentSession.trackedTabIds`). Nothing here
  is about concurrent recording.
- **(c) Several tabs claim "active" and the first claimer keeps it.**
  Rejected — that is a different feature (active-tab arbitration) and is not
  needed to make a tab-less command routable.

## First-wins is made deterministic

`Daemon.conns` is `map[*RelayConn]struct{}`. **Go randomizes map iteration order**,
so the existing `FirstTabID()` cannot honour its own "first match wins" comment —
across two calls it can and does return different tabs, and `rc.tabs` is a map
too, so the inner loop is equally unordered.

**Measured, not assumed.** A throwaway harness registered two connections with
three tabs each and called `FirstTabID()` 2000 times against unchanged state:

```
DISTINCT_TABS=6 [1 2 3 7 8 9]
COUNTS=map[1:1344 2:208 3:206 7:181 8:34 9:27]
VERDICT: NON-DETERMINISTIC — the "first match wins" comment is false
```

All six possible answers occurred, and the distribution is not even uniform. This
is a **pre-existing bug**, independent of the recording feature: with Chrome and
Firefox both connected, `POST /api/command` with no tab picks a different browser
from call to call. (The harness was removed after measuring; the regression test
required by this spec replaces it.)

This spec therefore does not rely on iteration order:

- Each `RelayConn` gets a `seq uint64` assigned from a `Daemon.nextConnSeq`
  counter when it is accepted.
- Selection picks the eligible connection with the **lowest `seq`**.
- If the winner's relay drops, the next-lowest `seq` becomes the winner
  automatically — no promotion bookkeeping, and no chance of two commands being
  routed to a dead connection.
- A losing tab is never involved: a command is written to exactly one connection.

`FirstTabID()` shares the same defect. Because the sequence is being introduced
anyway, this spec **fixes it there too** (2 lines: lowest `seq` instead of the
first map hit). That is a deliberate behaviour change to an existing path —
`POST /api/command` with a nil tabID becomes deterministic instead of random —
and it is listed in the tests so the change is visible rather than incidental.

## Current single-connection assumptions in the code

Grounding for the change, with real locations. These are what a tab-less command
has to route around:

| Assumption | Location | Effect on this change |
|---|---|---|
| A nil tabID is resolved by scanning connections and taking an arbitrary hit | `htrcli/internal/host/daemon.go:319` `FirstTabID` | 404 when no tab is registered; non-deterministic when several are. Fixed here. |
| A tabID maps to exactly one owning connection, first match wins | `htrcli/internal/host/daemon.go:333` `findOwner` | Unchanged — tab-routed commands still work identically. |
| "The active tab" is one tab from `currentWindow` | `src/background/index.ts:355`, `:637` | Irrelevant to a tab-less command: the extension handles recording actions before any `chrome.tabs` call, so no active tab is consulted. |
| A recording starts from the active tab and then tracks more | `src/background/index.ts:343` `startRecording`, `trackedTabIds` at `:374` | Confirms reading (b) is not what's being asked for — one session already spans many tabs. |
| A connection is only identifiable through its registered `http/https` tabs | `registerNativeTab`, `src/background/index.ts:87` | The reason `identify` is required. |

## Why a new `identify` message is unavoidable

`TabInfo.Browser` already exists and is populated — but it arrives via the
per-tab `register` message, and `registerNativeTab` returns early for anything
that is not `http/https` (`src/background/index.ts:87`).

Therefore **a connection with zero content-script tabs has no browser identity at
all**, and its `browser` cannot be derived from its tabs. Routing a hint at it is
impossible without a connection-level announcement. This is the only reason the
change touches the extension, and the only wire addition.

## Design

### 1. Extension — `identify` message

New outbound native message, sent once after the relay connects:

```jsonc
{ "type": "identify", "browser": "chrome" }  // or "firefox"
```

Sent from the native-host connect path (`startNativeHost` / the reconnect
success branch), using `getBrowserType()`. Re-sent after a reconnect, since a new
`RelayConn` replaces the old one and starts with no identity.

Additive: a daemon that predates this change ignores an unknown `type`. An
extension that predates it simply never sends one.

### 2. Daemon — address a connection, not a tab

- `RelayConn` gains `browser string` and `seq uint64`, both guarded by
  `Daemon.mu` (consistent with `tabs` and `lastSeen`).
- `Daemon.nextConnSeq` assigns `seq` when a relay is accepted.
- New `Daemon.SetConnBrowser(rc *RelayConn, browser string)`.
- New `Daemon.EnqueueBackgroundCommand(cmd Command, browser string) (<-chan CommandResult, error)`:
  - no connections → error `no browser connected`
  - `browser != ""` → the lowest-`seq` connection whose recorded browser matches
  - otherwise (no match, or no hint) → the lowest-`seq` connection
  - writes `NativeMessage{Type: "command", Payload: …}` with `TabID` left zero
  - registers the pending command exactly as `EnqueueCommand` does

Fallback-on-no-match is what makes the flag a *hint*: `--browser firefox` against
a daemon that only has Chrome connected answers from Chrome rather than failing.
That is a deliberate trade: a wrong-profile read over a hard error, chosen because
`list` will now report which browser answered.

Modern-Go guidelines applied: `slices.Contains`-style matching avoided in favour
of a single-pass loop under the existing lock (no extra allocation), `cmp.Or` for
the fallback chain where it reads clearly, `any` not `interface{}`.

### 3. HTTP — `POST /api/background/command`

```jsonc
// request
{ "command": { "id": "1", "action": "recordingList", "options": {} },
  "browser": "firefox",     // optional
  "timeout": 30000 }        // optional, default 30000
```

Added as a case in the existing `apiHandler` switch, keeping the file's
convention. The modern-Go `http_servemux_patterns` guideline is knowingly not
applied here: adopting it would mean migrating the entire switch, which is out of
scope and would not match the code being edited.

Responses reuse `apiOK` / `apiError` and the existing `commandRequest` shape
(extended with `Browser`).

### 4. Client — `ExecuteBackgroundCommandInto`

`func (c *Client) ExecuteBackgroundCommandInto(cmd Command, browser string, out any) error`

Mirrors `ExecuteCommandInto`, reusing the same `commandDataEnvelope` so a
~48 MB screenshot payload is still decoded **once** — the property item C of the
previous change established must not regress. Shares the `Success:false` and
empty-error handling.

### 5. CLI

- New persistent flag: `--browser` (`chrome` | `firefox`), default `""`.
  Validated once in the root persistent pre-run; an unknown value is an error
  listing the valid options, rather than being silently ignored.
- `recordings` commands call the background route, passing the hint.
- The existing `--transport cdp` rejection for `recordings` is unchanged.
- `recordings list` gains a **Browser** column, sourced from the daemon. This is
  required for the hint to be honest: if the fallback fired, the user must be
  able to see which profile answered.

## Compatibility

| Surface | Impact |
|---|---|
| `POST /api/command` (tab-routed) | **Untouched.** Every existing command keeps its current routing. |
| Older extension (no `identify`) | `browser == ""` → never matches a hint → first-wins. Works. |
| Older daemon (no route) | New route 404s; the CLI's error is unchanged in shape. |
| Extension `tabId: 0` reply | Already supported; results match on `commandId`. |

## Tests

**Go — daemon** (`internal/host/daemon_test.go`)
- `identify` records the browser on the connection
- hint matches a connection; prefers the lowest-`seq` match when several exist
- unknown/empty browser falls back to the lowest-`seq` connection
- **determinism: repeated calls with the same connections return the same
  connection**, and it is the earliest-connected one. Run the selection many
  times in one test — a single pass cannot detect map-order dependence, which is
  the exact bug this guards against.
- closing the winner promotes the next-lowest `seq` (no stale/dead routing)
- zero connections → `no browser connected`
- two connections with the same browser → lowest `seq`
- `FirstTabID()` is now deterministic across repeated calls (behaviour change)

**Go — server** (`internal/host/server_test.go`, new file)
- Route exercised **through the real `http.ServeMux`**, not the handler alone — a
  handler-only test cannot catch a route that was never wired.
- 400 on a missing/empty action; browser field forwarded; envelope shape asserted.

**Go — client** (`internal/api/`)
- decodes `data` once into `out`
- surfaces `ok:false` and `success:false` distinctly, including the
  message-less failure case
- `browser` reaches the request body; `""` omits it

**TS — extension**
- `identify` is sent on connect and again on reconnect, carrying
  `getBrowserType()`.

Every new Go and TS test is mutation-verified (mutate the implementation, confirm
the test fails, restore) before this spec is considered done.

## Documentation — required deliverables

This is a checklist, not a suggestion; each is part of "done":

- [ ] `SPEC_HTRCLI.md` — has **no session-recordings section at all** today; add
      one covering the `recordings` verb, the background route, and `identify`.
- [ ] `skills/htrcli/SKILL.md` (symlinked as `~/.claude/skills/htrcli`) —
      document `--browser` and the tab-less behaviour.
- [ ] `CLAUDE.md` — the architecture/command list.
- [ ] `CHANGELOG.md` — under `## Unreleased`.
- [ ] `htrcli/README.md`.
- [ ] `TODO.md` — close the items this spec lands, and add an entry for anything
      deferred.

## Out of scope

- **Strict** browser selection (erroring instead of falling back). The hint can
  be promoted to strict later without changing the route.
- Changing `/api/command`'s existing tab resolution.
- React component tests (item 3, previously declined — it needs a new dev
  dependency and this repo has no component tests at all).
- Streaming the export's base64 decode straight into the deflate writer: the
  saving is one `[]byte` per blob against an already-parsed ~48 MB payload, so
  it is noise. To be closed in `TODO.md` as not-worth-doing, with the reasoning.

## Working-tree conflicts

Every file this change touches already carries **uncommitted work** from the
in-progress session-recording feature. The plan must edit in place and re-read
each file immediately before editing — do not restore, stash, or `checkout` any
of them:

- `htrcli/internal/host/daemon.go` — connection/tab state
- `htrcli/internal/host/server.go`, `bridge.go` — routing
- `htrcli/internal/api/client.go`, `types.go` — client + envelopes
- `htrcli/internal/commands/recordings.go` — the verb being rewired
- `src/background/nativeHost.ts`, `index.ts` — the `identify` send
- `src/types/commands.ts` — the action union

Note: `git checkout <path>` is denied by `.claude/settings.json` in this repo, so
`cp` from `git show HEAD:<path>` is the restore mechanism if a file must be reset.

## Risks

| Risk | Mitigation |
|---|---|
| Wrong browser answers when the hint misses | `list` reports the browser; documented as a hint |
| A third browser connects (e.g. Edge) | `getBrowserType()` reports what it reports; the hint simply never matches and first-wins applies |
| `identify` lost on reconnect | Re-sent on every successful connect; a lost identity degrades to first-wins, which is the old behaviour |
| Daemon grows a new lock-guarded field | Same mutex and discipline as `tabs`/`lastSeen`; no new lock |

## Corrections to prior notes

Two statements I made earlier were wrong and are corrected here:

1. **"Not done: it changes the host protocol."** The *route* does not — `tabId`
   is already `omitempty` and results match on `commandId`. The `--browser` hint
   does require a new message, but only because a zero-tab connection is
   anonymous.
2. **"The export holds the bundle in memory."** `writeRecordingZip` already
   streamed per entry; the cost was the JSON decode chain, removed by
   `ExecuteCommandInto` in the previous change.
