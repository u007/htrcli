# Part 2 — Wire protocol and tab-less route

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Let the extension announce which browser it is, teach the daemon to record that, expose a tab-less `POST /api/background/command` route, and add a client method that decodes a response in one pass.

**Spec:** `SPEC_BACKGROUND_COMMANDS.md`. Read the INDEX for global constraints.

**Scope note:** Consumes Part 1's connection identity and tab-less enqueue. Adds no CLI surface — Part 3 does that. Every change here is backwards compatible: an old daemon ignores the unknown `identify` type, and an old extension simply never sends it.

---

## Task 2.1: Extension announces its browser on connect

**Files:**
- Modify: `src/background/nativeHost.ts`
- Create: `src/background/nativeIdentify.test.ts`

- [ ] Read `src/background/nativeHost.ts` around `connectNative`, `confirmConnected`, and `sendToNative` to find where a **successful** connection is confirmed.
- [ ] Determine whether `confirmConnected` fires once per connection or once per inbound message. If it can fire repeatedly, do **not** hook there — hook the one-shot successful-connect path, so `identify` is sent exactly once per native port. A repeated `identify` would be harmless to the daemon but would add needless traffic on every message.
- [ ] Add an outbound native message of type `identify` carrying the browser value from the existing exported `getBrowserType()` in this same file. **Reuse that function** — do not add new UA or manifest sniffing, and do not add a second place that decides which browser this is.
- [ ] The message must be sent again after every reconnect, because each reconnect creates a new daemon-side connection that starts with no identity.
- [ ] Write a test asserting `identify` is sent with the value `getBrowserType()` returns, and that it is sent again on a second connect.
- [ ] The test needs `chrome.runtime.connectNative` and the native `sendNativeMessage` stubbed. Follow whatever harness the existing `src/background/nativeHost.test.ts` uses rather than inventing a new one.
- [ ] Run `bun test src/background/nativeIdentify.test.ts` and confirm it fails first, then passes.
- [ ] Mutation-verify: make the handler send an empty browser, confirm the test fails, revert.
- [ ] Run `bun run check:fix`, `bun test`, and `bun run firefox:typecheck`. Firefox must behave identically — `getBrowserType` already distinguishes them via `chrome.debugger`.

---

## Task 2.2: Daemon records the announced browser

**Files:**
- Modify: `htrcli/internal/host/bridge.go`
- Test: `htrcli/internal/host/bridge_test.go`

- [ ] Read the inbound message loop in `bridge.go` — the switch already handles `register`, `command_result`, and `heartbeat`.
- [ ] Add a case for `identify` that extracts the browser value and calls the setter added in Part 1. **Ignore it silently if the value is empty or unrecognised**: an extension sending junk must not disconnect the relay or fail the connection.
- [ ] Do not register a tab. The whole point is that identity arrives without a tab.
- [ ] Add a test in the existing `bridge_test.go` that pushes an `identify` native message through the relay and asserts the connection's recorded browser.
- [ ] Add a second test for an empty or unknown browser value asserting the connection is untouched and still usable.
- [ ] Confirm the connection's liveness bookkeeping still runs for this message type, consistent with the other cases.
- [ ] Run `cd htrcli && go test -count=1 ./internal/host/`.
- [ ] Mutation-verify: make the case a no-op, confirm the test fails, revert.

---

## Task 2.3: The tab-less HTTP route

**Files:**
- Modify: `htrcli/internal/host/server.go`
- Create: `htrcli/internal/host/server_background_test.go`

**Why a new file:** the route must be exercised through the **real `http.ServeMux`**, not by calling the handler directly. A handler-only test passes even when the route was never wired, which is exactly the failure this task could otherwise ship.

- [ ] Re-read `apiHandler` in `server.go` to see the switch and the existing `commandRequest` decoding.
- [ ] Extend `commandRequest` with an optional `Browser` field. Do not create a second request type; the bodies are otherwise identical.
- [ ] Add the `POST /api/background/command` case to the existing switch, mirroring the tab-routed command case: validate a non-empty action, apply the same default timeout, delegate to the tab-less enqueue from Part 1 with the browser hint, and reply using the same success/error helpers.
- [ ] The connection lookup must **not** go through `FirstTabID`. That is the entire point of the route.
- [ ] Add a command-id fallback: if the caller supplies no id, generate one, as the tab-routed path does.
- [ ] Write tests that build the server and issue **real HTTP requests** through it, covering:
  - a valid request reaches the connection and returns the extension's result
  - the browser hint from the body is forwarded to the daemon
  - a missing or empty action is rejected as a bad request
  - no connected browser returns the `no browser connected` error, not a hang
  - the route exists — assert it by path, which a handler-only test cannot do
- [ ] Confirm the auth middleware still wraps this route: it must inherit the same bearer-token and IP handling as every other `/api/` route, because it is not registered outside the `/api/` prefix.
- [ ] Run `cd htrcli && go test -count=1 ./internal/host/ ./internal/api/`.
- [ ] Mutation-verify: rename the route so it stops matching, confirm the "route exists" test fails, revert. This is the mutation that proves the test has teeth.
- [ ] Run `gofmt -l htrcli/internal/host/` and `go vet ./...`.

---

## Task 2.4: Client method that decodes in one pass

**Files:**
- Modify: `htrcli/internal/api/client.go`
- Create: `htrcli/internal/api/client_background_test.go`

- [ ] Read the existing `ExecuteCommandInto` and its `commandDataEnvelope`. **Reuse that envelope type** — do not define a second one. The single-pass decode is what keeps a ~48 MB screenshot payload from being copied three extra times, and a parallel envelope risks drifting from it.
- [ ] Add a background-route variant of the command method that takes the browser hint and decodes the response's `data` straight into the caller's type.
- [ ] Omit the browser field from the request body when the hint is empty, so the wire format for existing callers is unchanged.
- [ ] Copy the failure handling from the existing method exactly, including the case where the extension reports `success:false` **with an empty error** — that must still produce a non-empty message naming the action, or it reads like success at the call site.
- [ ] Add tests mirroring the existing `ExecuteCommandInto` tests: correct decode into a typed value, the two distinct failure shapes, the empty-error case, a success carrying no data, a deliberately mismatched output type producing a decode error, and the hint appearing in (and being omitted from) the request body.
- [ ] Run `cd htrcli && go test -count=1 ./internal/api/`.
- [ ] Mutation-verify: make the method marshal the payload back to bytes before decoding — the old, wasteful shape — and confirm the existing allocation-sensitive expectations or an added allocation bound fails. If an allocation bound is added, keep it generous enough not to be flaky.

---

## Part 2 completion check

- [ ] `cd htrcli && go build ./... && go vet ./... && go test -count=1 ./...` all pass
- [ ] `gofmt -l htrcli/internal` reports only the pre-existing `publish.go`
- [ ] `bun run check`, `bun run test`, `bun run firefox:typecheck` all pass
- [ ] The route is proven reachable through the real mux (the rename mutation was observed failing)
- [ ] Every mutation applied was observed failing at least one test, then reverted
- [ ] Report confirms backwards compatibility in both directions: an old daemon ignores `identify`, an old extension still works
