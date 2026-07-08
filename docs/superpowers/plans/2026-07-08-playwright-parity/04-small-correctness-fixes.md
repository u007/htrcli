# Part 4: Small Correctness Fixes (`wait`, `scrollTo`, `evaluate`)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Three silent-failure traps in the command executor become loud and correct: `wait` errors on timeout instead of returning null-as-success, `scrollTo` finishes scrolling before it responds, and `evaluate` accepts multi-statement and async scripts.

**Architecture:** All changes live in `src/contentScript/commandExecutor.ts` (plus its element-finder dependency), behind the existing action router — no wire-format, transport, or manifest changes. Tests use the happy-dom harness (a `bunfig.toml` test preload registering happy-dom globals, delivered before this part; if executing this part standalone, create that harness first — see the steps in Task 4.1).

**Tech Stack:** TypeScript content script, Bun test + happy-dom.

## Global Constraints (inherited, restated)

- No silent fallbacks or null-as-success; errors name the action and the unmet condition.
- Nothing Chrome-only; Firefox shares this code (`bun run firefox:typecheck` must pass).
- Verify after the part: `bunx tsc --noEmit`, `bun test`, `bun run build`, `bun run firefox:typecheck`.

---

### Task 4.1: `wait` action errors on timeout

**Files:**
- Modify: `src/contentScript/commandExecutor.ts` (`handleWait`)
- Modify: `src/contentScript/elementFinder.ts` (only if the `waitForAppear` gate is still consulted there)
- Test: `src/contentScript/commandExecutor.test.ts`

**Current defects being fixed:** (1) `waitForElement` only actually waits when the caller sets `target.waitForAppear` — otherwise it returns immediately; for the `wait` action, waiting is the entire point, so the gate must not apply. (2) On timeout it resolves `null`, which `executeCommand` wraps as `success: true, data: null` — callers cannot distinguish "element appeared" from "gave up".

**New behavior:** `wait` always waits (up to `options.timeout`, default 5000ms) and, on timeout, throws an error naming the selector and the elapsed time, which `executeCommand` already converts to `success: false`. On success it returns the element info as today. If the DOM-harness preload (`bunfig.toml` `[test]` preload registering `@happy-dom/global-registrator`, dev dependency pinned to a major) does not exist yet, add it as the first step.

**Steps:**

- [ ] Write failing tests via `executeCommand`: (a) `wait` for an element added 100ms later returns `success: true` with element info; (b) `wait` for a never-matching selector with a 200ms timeout returns `success: false` with the selector named in the error; (c) `waitForAppear` absent from the target does not disable waiting.
- [ ] Implement; tests pass; grep the repo for other `waitForElement` callers to confirm none relied on the null-on-timeout contract.
- [ ] Full verify, `bun run check:fix`, commit: `fix: wait action fails loudly on timeout`.

---

### Task 4.2: `scrollTo` scrolls instantly and settles before responding

**Files:**
- Modify: `src/contentScript/commandExecutor.ts` (`handleScrollTo`)
- Test: `src/contentScript/commandExecutor.test.ts`

**Current defect being fixed:** `scrollIntoView({ behavior: "smooth" })` returns before the animation finishes, so a screenshot taken by the very next command captures mid-scroll.

**New behavior:** scroll with instant behavior, centered block, then wait for the scroll to settle before resolving — settle defined as scroll position stable across two consecutive animation frames (bounded by a short hard cap ~500ms so a page with its own scroll animations can't hang the command). The handler becomes async; the action router already awaits handlers.

**Steps:**

- [ ] Write failing tests: (a) `scrollTo` resolves with the target element inside the viewport (happy-dom exposes scroll positions; assert the position changed and is stable); (b) resolves within the hard cap even if positions keep changing.
- [ ] Implement; tests pass.
- [ ] Full verify, `bun run check:fix`, commit: `fix: scrollTo settles before responding`.

---

### Task 4.3: `evaluate` supports multi-statement and async scripts

**Files:**
- Modify: `src/contentScript/commandExecutor.ts` (`handleEvaluate`)
- Test: `src/contentScript/commandExecutor.test.ts`

**Current defects being fixed:** (1) the script is wrapped as a single parenthesized expression, so any multi-statement script (`const a = …; return a`) is a SyntaxError; (2) promise-returning expressions are returned unawaited, serializing as an empty object; (3) the isolated-world semantics (cannot see page JS globals) are undocumented, which reads as a bug to callers.

**New behavior:** compile the script in two deterministic modes — first as a single expression (preserving today's `document.title`-style usage), and if that *compilation* throws a SyntaxError, compile it as a function body where the caller uses explicit `return` (compile-time mode selection, not a runtime fallback; add a comment stating exactly this distinction). Execute via an async function so `await` works inside scripts, and await the result before returning it. Runtime errors from the script propagate unchanged. Isolated-world semantics get documented, not changed — `debuggerEval` is the page-context tool.

**Steps:**

- [ ] Write failing tests via `executeCommand`: (a) expression scripts still work (`document.title`); (b) multi-statement script with `return` works; (c) script using `await Promise.resolve(42)` returns 42, not `{}`; (d) a script that throws yields `success: false` with the script's own error message; (e) a genuinely invalid script (syntax error in both modes) yields `success: false` with a SyntaxError message.
- [ ] Implement; tests pass.
- [ ] Full verify, `bun run check:fix`, commit: `fix: evaluate handles multi-statement and async scripts`.

---

### Task 4.4: Documentation

**Files:**
- Modify: `skills/htrcli/SKILL.md` — `eval` section: multi-statement/`await` now supported; add one sentence that `eval` runs in the extension's isolated world (page variables invisible) and that `htcli` page-context evaluation goes through the `debuggerEval` action.
- Modify: `htcli/README.md` — same notes in the eval/interaction sections; mention `wait` now errors on timeout (breaking change for callers that treated null as "not found, keep going" — call it out explicitly).

**Steps:**

- [ ] Update both docs; commit: `docs: eval semantics, wait timeout behavior`.
