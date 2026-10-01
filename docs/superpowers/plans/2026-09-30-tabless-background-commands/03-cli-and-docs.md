# Part 3 — CLI surface and documentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Give the CLI an advisory `--browser` hint, point the `recordings` verb at the tab-less route, make `recordings list` report which browser actually answered, and update every documentation surface.

**Spec:** `SPEC_BACKGROUND_COMMANDS.md`. Read the INDEX for global constraints and the Review Focus list.

**Scope note:** Consumes Parts 1 and 2. This is the part where the hint becomes honest — if the fallback fired because the named browser was not connected, the user must be able to see that.

---

## Task 3.1: Report the answering browser from the extension

**Files:**
- Modify: `src/background/recordingCommands.ts`
- Modify: `src/background/recordingCommands.test.ts`
- Modify: `htrcli/internal/api/types.go`

**Why the extension, not the daemon:** the daemon knows which connection it picked, but the extension knows what it *is*. The extension already has `getBrowserType()` in the same background bundle, so it should report its own identity. This also keeps the field correct if the daemon's fallback logic is later tightened to strict selection.

- [ ] Add a `browser` field to the `recordingList` result shape in `recordingCommands.ts`, sourced from `getBrowserType()`. Import it rather than sniffing again.
- [ ] **Only** `recordingList` needs it. Do not add it to every action's result — the point is to make the `list` output explain which profile answered.
- [ ] Update the Go mirror `RecordingListData` in `htrcli/internal/api/types.go` with the matching field. Without this the field is silently dropped at decode time and the column would always be blank — a failure that looks like a UI bug.
- [ ] Add a test in `recordingCommands.test.ts` asserting the list result carries the expected browser value.
- [ ] Mutation-verify: remove the field from the Go type, confirm the corresponding Go-level assertion fails, revert. This catches the silent-drop case, which is the easy one to ship.
- [ ] Run `bun test src/background/recordingCommands.test.ts` and `cd htrcli && go test -count=1 ./internal/commands/`.

---

## Task 3.2: The `--browser` flag

**Files:**
- Modify: `htrcli/internal/commands/root.go`

- [ ] Re-read `root.go` to see how the existing persistent flags (`--transport`, `--tab`, `--context`) are declared and validated. Follow that pattern exactly.
- [ ] Add a persistent `--browser` string flag accepting `chrome` or `firefox`, defaulting to empty (meaning "no preference").
- [ ] Validate it in the persistent pre-run, alongside the existing `--tab` validation. An unrecognised value must be a clear error listing the valid options — **not** silently ignored, which would make a typo look like a working first-wins call.
- [ ] Empty means no hint. Do not default it to a browser; defaulting would silently override first-wins.
- [ ] Add a flag-parsing test for: absent, `chrome`, `firefox`, and an invalid value. Note that a persistent flag mutates shared package state, so the test must restore the previous value.
- [ ] Confirm the flag does not affect `--transport cdp`: the existing rejection of `recordings` over CDP must still fire, and a browser hint must not become a way around it.
- [ ] Run `cd htrcli && go test -count=1 ./internal/commands/`.

---

## Task 3.3: Point the `recordings` verb at the tab-less route

**Files:**
- Modify: `htrcli/internal/commands/recordings.go`
- Modify: `htrcli/internal/commands/recordings_test.go`

- [ ] Re-read `recordingData` and `sendRecordingCommand` in `recordings.go`. Both currently post to the tab-routed route with a nil tab.
- [ ] Switch the recording path to the tab-less client method from Part 2, passing the `--browser` value as the hint.
- [ ] Keep the existing `--transport cdp` rejection, and keep routing that rejection through a single place so the message keeps one home.
- [ ] Update the comment block that documents the "needs one connected http/https tab" limitation — it is now **false** and must be corrected in the same edit, not left to rot.
- [ ] Add a `Browser` column to the `recordings list` table output, using the field added in Task 3.1. Keep the table sorted newest-first and paginated exactly as it is today; this plan changes nothing about ordering.
- [ ] Make sure the column also appears in JSON output mode, since the machine-readable path is what a caller would parse.
- [ ] Add tests: the hint reaches the request; the list table and JSON both surface the browser; the CDP rejection still fires with a browser hint set.
- [ ] Mutation-verify: make the recordings path go back to the tab-routed route, confirm a test fails, revert.
- [ ] Run `cd htrcli && go test -count=1 ./internal/commands/`.

---

## Task 3.4: Documentation — required deliverables

Each item is part of "done", not a follow-up.

- [ ] `SPEC_HTRCLI.md` — it has **no session-recordings section at all** today. Add one covering the `recordings` verb, the tab-less route, the `identify` message, and the `--browser` hint. Match the file's existing structure and voice.
- [ ] `skills/htrcli/SKILL.md` (symlinked as `~/.claude/skills/htrcli`) — document `--browser` as a **hint with first-wins fallback**, and update the note that says recordings need a connected `http/https` tab.
- [ ] `CLAUDE.md` — the architecture diagram and command list: add the background route and the `identify` message.
- [ ] `htrcli/README.md` — the endpoint and flag reference.
- [ ] `CHANGELOG.md` — a new entry under `## Unreleased`, including the `FirstTabID` determinism fix as a user-visible behaviour change, since it alters which browser a tab-less command reaches.
- [ ] `TODO.md` — close the session-recordings items this work lands, and **add an entry for anything still deferred**. Specifically, record that the export's base64→deflate streaming was considered and closed as not-worth-doing, with the reasoning (the saving is one `[]byte` per blob against an already-parsed ~48 MB payload), and that React component tests remain uncovered because the repo has no component-test harness and the user declined adding one.
- [ ] Re-check every `file:line` anchor you wrote into any doc against the final source. Anchors drift as soon as code is added — an anchor written before the last task is likely wrong by the end.

---

## Task 3.5: Full verification

- [ ] From the repo root: `bun run check`
- [ ] `bun run test`
- [ ] `bun run build`
- [ ] `bun run firefox:typecheck`
- [ ] `bun run firefox:build`
- [ ] From `htrcli/`: `go build ./...`, `go vet ./...`, `go test -count=1 ./...`
- [ ] From the repo root: `make htrcli-build`
- [ ] `gofmt -l htrcli/internal htrcli/cmd` reports only `publish.go`
- [ ] `./htrcli/bin/htrcli recordings --help` shows the `--browser` flag and the updated one-line description
- [ ] `./htrcli/bin/htrcli recordings list --help` mentions the browser hint
- [ ] Confirm the built extension still contains the `identify` send, and that the Firefox build does too (they share `src/`)
- [ ] Confirm **no** scratch or throwaway files remain (`zz_*`, ad-hoc parity scripts)

---

## Part 3 completion check

- [ ] All gates in Task 3.5 pass, with output shown
- [ ] Every mutation applied across all three parts was observed failing at least one test, then reverted
- [ ] The six documentation files are updated, including the now-false "needs a connected tab" limitation wherever it appears
- [ ] `TODO.md` records every deferred item, and the deferred items are named in the final report to the user
- [ ] The final report states plainly: what now works that did not before, the `FirstTabID` behaviour change, and anything still deferred
- [ ] Nothing was committed (the user did not ask for a commit)
