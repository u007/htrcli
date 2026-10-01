# Part 1 — Daemon connection identity and deterministic selection

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Give every relay connection a monotonic sequence number and a recorded browser identity, add a tab-less command enqueue, and make `FirstTabID()` deterministic.

**Spec:** `SPEC_BACKGROUND_COMMANDS.md`. Read the INDEX for global constraints and the measured evidence that motivates the `seq` field.

**Scope note:** This part touches **only** `htrcli/internal/host/`. It adds no HTTP route and no CLI surface, so it is independently testable and independently reviewable. Parts 2 and 3 consume what lands here.

---

## Task 1.1: Prove the existing non-determinism with a regression test

**Files:**
- Create: `htrcli/internal/host/daemon_determinism_test.go`

**Why first:** every later task depends on `seq` being the thing that fixes ordering. If the test for that does not exist first, there is nothing to prove the fix works, and a green suite after the change proves nothing.

- [ ] Read `htrcli/internal/host/daemon_test.go` to learn the existing test idiom. Note that a connection must be created with `d.AddConn(...)` — `RegisterTab` alone writes `rc.tabs` but never puts the connection into `d.conns`, which is the map being iterated. Getting this wrong makes `FirstTabID` return `false` and the test appears to pass for the wrong reason.
- [ ] Write a test that registers two connections, each with three tabs, then calls `FirstTabID()` many times (at least 2000) and asserts every call returns the **same** tab id.
- [ ] Name the test so the failure is self-explaining, e.g. `TestFirstTabIDIsDeterministicAcrossRepeatedCalls`.
- [ ] Run `cd htrcli && go test -count=1 -run TestFirstTabIDIsDeterministic ./internal/host/` and **confirm it FAILS**. Record the failure output in your report.
- [ ] Add a comment above the test explaining that a single call cannot detect map-order dependence — the loop count is the point.

**Do not** fix anything in this task. A failing test is the deliverable.

---

## Task 1.2: Add `seq` and `browser` to `RelayConn`

**Files:**
- Modify: `htrcli/internal/host/daemon.go`

- [ ] Run the Modern Go guidelines CLI for this file: `sh "<skill-dir>/scripts/run-tool.sh" list --file-path htrcli/internal/host/daemon.go` and read the full output.
- [ ] Re-read the `RelayConn` struct and the `Daemon` struct. Both new fields are guarded by the existing `Daemon.mu`; **do not introduce a new lock**.
- [ ] Add a `browser string` field and a `seq uint64` field to `RelayConn`, with comments stating they are guarded by `Daemon.mu` exactly like `tabs` and `lastSeen`.
- [ ] Add a `nextConnSeq uint64` counter to `Daemon`.
- [ ] Assign the sequence inside `AddConn` (the single place a `RelayConn` is constructed and registered), and initialise `browser` to the empty string. Use `range` over an integer where a loop index is needed, per the guidelines.
- [ ] Confirm `AddConn` still takes and releases the lock exactly once and that no caller can observe a zero sequence.
- [ ] Run `cd htrcli && go build ./... && go test -count=1 ./internal/host/` — the determinism test from Task 1.1 **still fails** (nothing selects on `seq` yet). That is expected.
- [ ] Run `gofmt -l htrcli/internal/host/`.

---

## Task 1.3: Make `FirstTabID()` deterministic

**Files:**
- Modify: `htrcli/internal/host/daemon.go`
- Test: `htrcli/internal/host/daemon_determinism_test.go`

**Why in scope:** the user explicitly kept this. It is a **behaviour change** to an existing path, so it must be visible rather than incidental.

- [ ] Re-read `FirstTabID` and its four callers in `htrcli/internal/host/server.go` (the `handleCommand` nil-tab branch, and the other three). Note in your report that this widens beyond the recording feature: every route that resolves a nil tab is affected.
- [ ] Rewrite the selection to pick the lowest-`seq` connection that has at least one tab, returning a tab from that connection. Do **not** rely on map iteration order for either the connection choice or the tab choice within it.
- [ ] Update the function's doc comment: it currently claims "first match wins", which was false. The new comment must state that it returns a tab from the earliest-connected eligible connection and is deterministic.
- [ ] Leave `findOwner` (same file) **unchanged** — it resolves a specific tabID, where there is only one correct connection, and its first-match behaviour is unrelated.
- [ ] Run `cd htrcli && go test -count=1 ./internal/host/`. The Task 1.1 determinism test must now **PASS**.
- [ ] Run the full host suite: `cd htrcli && go test -count=1 ./...`. Existing `FirstTabID` callers have tests in `server_test.go` and `daemon_test.go`; if any asserted a specific tab that was previously only stable by luck, fix the **test's expectation** to the now-documented deterministic answer, and say so explicitly in your report.
- [ ] Mutation-verify: temporarily restore the map-order behaviour, confirm the determinism test fails, then restore. Report the result.

---

## Task 1.4: Add connection-level browser identity and tab-less enqueue

**Files:**
- Modify: `htrcli/internal/host/daemon.go`
- Create or extend: `htrcli/internal/host/daemon_background_test.go`

- [ ] Add a setter for a connection's browser that takes `Daemon.mu` and stores only a non-empty value. An empty value must mean "unknown", and must not clobber a previously announced identity.
- [ ] Add an unexported helper that selects the target connection for a hint: given the connection set and a hint string, return the lowest-`seq` connection whose recorded browser matches, or the lowest-`seq` connection overall when the hint is empty **or when nothing matches**. Write the fallback as the deliberate behaviour it is, with a comment saying a hint is advisory and a wrong-profile answer is preferred over a hard error because `recordings list` will report which browser answered.
- [ ] Add the tab-less enqueue, mirroring the existing `EnqueueCommand` structure: resolve the connection, register the pending command under the command id, write a native `command` message with the tab id left at zero, and clean up the pending entry if the write fails. Return a channel that receives the result, and a clear `no browser connected` error when the connection set is empty.
- [ ] Confirm the pending-command bookkeeping is identical to `EnqueueCommand`'s, since result delivery keys on the command id and not the tab id.
- [ ] Add tests covering, in this order:
  - zero connections → the `no browser connected` error
  - one connection, no hint → that connection
  - two connections, no hint → the earliest, asserted over many repeats
  - a hint matching one connection → that connection, even when it is not the earliest
  - a hint matching **nothing** → the earliest connection, not an error
  - two connections with the same browser → the earliest
  - closing the winning connection → the next-earliest becomes the winner
  - a failed write removes the pending entry, matching `EnqueueCommand`'s behaviour
- [ ] Run `cd htrcli && go test -count=1 ./internal/host/` and `go vet ./...`.
- [ ] Mutation-verify the two load-bearing behaviours separately: (a) make selection ignore the hint, (b) make selection return the last connection instead of the earliest. Each must fail at least one test. Report both.
- [ ] Run `gofmt -l htrcli/internal/host/` — must be silent.

---

## Part 1 completion check

- [ ] `cd htrcli && go build ./... && go vet ./... && go test -count=1 ./...` all pass
- [ ] `gofmt -l htrcli/internal/host/` is silent
- [ ] The Task 1.1 test was observed **failing before** the fix and passing after (both recorded)
- [ ] Every mutation applied was observed failing at least one test, then reverted
- [ ] `git diff` shows no changes outside `htrcli/internal/host/`
- [ ] Report states plainly which existing `FirstTabID` callers changed behaviour, and whether any test expectation had to be corrected
