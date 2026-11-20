---
type: Gotcha
title: Runtime.evaluate's protocol timeout does not bound awaitPromise — race it in JS
description: CDP Runtime.evaluate's `timeout` only terminates synchronous execution; a never-settling awaited promise keeps the send pending and holds the debugger attach. cdpEvaluate races the round trip with a JS withTimeout (30 s in both the native-host eval and CDP_EVAL paths) so the caller's finally can detach.
resource: src/background/cdpEval.ts; src/background/nativeHost.ts; src/background/index.ts; src/background/cdpEval.test.ts
tags: [cdp, eval, timeout, debugger, attach, promise]
timestamp: 2026-11-19T03:54:00Z
status: active
---

# Eval timeout: race the send, don't trust the protocol field

`cdpEvaluate` (`src/background/cdpEval.ts`) sends `Runtime.evaluate` with `awaitPromise: true` and, when a timeout is supplied, the protocol's own `timeout` field.

The protocol field is **not** a bound on the whole round trip. It only terminates *synchronous* execution. A script such as `await new Promise(() => {})` never settles, the send promise never settles either, and the caller's `finally` never runs — so the shared debugger attach stays held and every later command on that tab fails with `Another debugger is already attached` until reload.

The fix is a JS-side race: `withTimeout(send(...), timeoutMs)` rejects with `cdpEvaluate: eval timed out after <n>ms`, which surfaces as a normal error reply and lets `finally` call `detachShared`. The protocol `timeout` is still forwarded (it bounds the synchronous loop), and `timeoutMs <= 0` means no JS bound.

Both call sites pass 30 s:

- Native-host eval — `EVAL_TIMEOUT_MS = 30000`, shortened only through the `__setEvalTimeoutForTests` seam (`src/background/nativeHost.ts`).
- `CDP_EVAL` — `CDP_EVAL_TIMEOUT_MS = 30000` (`src/background/index.ts`).

`src/background/cdpEval.test.ts` proves the gap: "rejects when the send promise never settles" uses a send that returns a pending promise and a 20 ms bound. The end-to-end guarantee lives in `src/background/nativeHost.test.ts` — "debuggerEval times out, replies with an error, and detaches exactly once". If a future change drops `withTimeout` and relies on the protocol `timeout`, those tests fail.
