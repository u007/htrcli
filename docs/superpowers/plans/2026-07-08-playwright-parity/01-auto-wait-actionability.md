# Part 1: Auto-Wait + Actionability for Interaction Actions

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every interaction action (click, fill, type, check, hover, …) waits — up to a timeout — for its target element to exist, be visible, and be enabled before acting, and returns a descriptive error naming the failed condition instead of an instant "Element not found".

**Architecture:** A new `waitForActionableElement` helper in the content script's element finder wraps the existing MutationObserver-based `waitForElement` and adds a poll for visibility/enabled state. All interaction handlers in the command executor switch from the instant `findElement` to this helper. Read-only inspection actions keep instant semantics on purpose.

**Tech Stack:** TypeScript content script, Bun test runner + happy-dom (new dev dependency, pinned major).

## Global Constraints (inherited, restated)

- `bun` only; Biome tabs/double quotes; `bun run check:fix` before commit.
- Default wait timeout must keep total command time well under the 30s transport ceilings.
- Errors must be loud and specific — no null-returns-as-success.
- Firefox shares this code unchanged; nothing here may use Chrome-only APIs.
- Verify after the part: `bunx tsc --noEmit`, `bun test`, `bun run build`, `bun run firefox:typecheck`.

---

### Task 1.1: DOM test harness (happy-dom)

**Files:**
- Modify: `package.json` (add `@happy-dom/global-registrator` as devDependency, pinned major)
- Create: `src/test/dom-preload.ts` (registers happy-dom globals)
- Create: `bunfig.toml` (or extend if present) with a `[test]` preload entry pointing at the preload file
- Test: `src/contentScript/elementFinder.test.ts` (created here with one smoke test)

**Interfaces:**
- Produces: a working `document`/`window` global inside `bun test`, so later tasks can create elements and assert on DOM behavior. Later tasks assume `bun test src/contentScript/elementFinder.test.ts` runs with DOM available.

**Steps:**

- [ ] Add the happy-dom registrator dev dependency with `bun add -d` (pin major version).
- [ ] Create the preload file that registers happy-dom globals, and wire it into `bunfig.toml` `[test]` preload. Confirm existing pure-logic tests are unaffected.
- [ ] Write one smoke test in `elementFinder.test.ts`: create an element in `document.body`, assert `findElement` locates it by CSS selector. Run it and confirm it passes.
- [ ] Run the full suite (`bun test`) — all 218+ existing tests still pass (the preload must not break pure-logic test files).
- [ ] `bun run check:fix`, commit: `test: add happy-dom DOM harness for content script tests`.

---

### Task 1.2: `waitForActionableElement` helper

**Files:**
- Modify: `src/contentScript/elementFinder.ts`
- Test: `src/contentScript/elementFinder.test.ts`

**Interfaces:**
- Consumes: existing `findElement(target)`, `waitForElement(target, timeoutMs)`, `getElementInfo(element)` (which already computes `visible` and `enabled`).
- Produces: `waitForActionableElement(target: TargetSelector, opts): Promise<Element>` where `opts` carries `timeoutMs` (default 5000) and a `requireEnabled` boolean (hover/scroll targets need not be enabled). It **throws** on timeout with a message naming the target and which condition failed — one of: not found, not visible, disabled. Later tasks (command executor, Part 3's coordinate prep, Part 4) call exactly this signature.

**Behavioral spec (describe, don't code):**
- Phase A — existence: reuse the MutationObserver wait so appearance is event-driven, not poll-driven. The current `waitForAppear` opt-in gate must NOT apply here — this helper always waits.
- Phase B — actionability: once the element exists, poll on a short interval until `getElementInfo` reports `visible` (and `enabled` when required) or the shared deadline expires. One deadline covers both phases.
- On timeout, the error message must distinguish "never appeared" from "present but hidden" from "present but disabled" — this is the difference between a wrong selector and a timing bug for the caller.

**Steps:**

- [ ] Write failing tests: (a) element added to the DOM 100ms after the call resolves successfully; (b) selector that never matches rejects with "not found" wording after a short test timeout; (c) element present but `display:none` rejects with "not visible" wording; (d) disabled button rejects with "disabled" wording when `requireEnabled` is set, resolves when it is not; (e) element that becomes visible 100ms in resolves. Run — all fail.
- [ ] Implement the helper per the behavioral spec. Run — all pass.
- [ ] Full verify (`bun test`, `bunx tsc --noEmit`), `bun run check:fix`, commit: `feat: waitForActionableElement with visibility/enabled checks`.

---

### Task 1.3: Switch interaction handlers to auto-wait

**Files:**
- Modify: `src/contentScript/commandExecutor.ts`
- Test: `src/contentScript/commandExecutor.test.ts` (new DOM-backed describe block)

**Interfaces:**
- Consumes: `waitForActionableElement` from Task 1.2.
- Produces: all interaction handlers become async and honor `command.options.timeout` (milliseconds, capped at 20000) as the wait budget. The wire format is unchanged — `options.timeout` already exists as a passthrough field.

**Scope of the switch:**
- Auto-wait + visible + enabled: `click`, `dblclick`, `rightclick`, `fill`, `type`, `clear`, `select`, `check`, `uncheck`, `pressKey`.
- Auto-wait + visible only (no enabled requirement): `hover`, `focus`, `blur`, `scrollTo`, `selectText`, `highlight`.
- Unchanged (instant, intentional probing semantics — add a code comment saying so): `find`, `findAll`, `isVisible`, `isEnabled`, `getValue`, `getAttribute`, `getText`, `getHTML`, `getOuterHTML`, `getBoundingBox`, `getComputedStyle`, `xpath`.

**Also in this task (same handler, same reviewer gate):** move `handleClick`'s `scrollIntoView` to *before* the event dispatch, with instant (non-smooth) behavior and centered block — clicking an off-viewport element currently dispatches events first and scrolls after, the reverse of correct order.

**Steps:**

- [ ] Write failing DOM tests through the public `executeCommand` entry: (a) `click` on an element that appears 100ms later succeeds; (b) `click` on a never-appearing selector returns `success:false` with the "not found" wording and a duration ≈ the timeout; (c) `fill` on a disabled input fails with "disabled" wording; (d) `options.timeout` of 200ms is respected (duration well under the 5s default); (e) `getText` on a missing element still fails instantly (probing semantics preserved). Run — all fail.
- [ ] Convert the listed handlers to await the helper; thread `options.timeout` through; reorder click's scroll. Run — all pass, plus the full suite.
- [ ] Full verify including `bun run build` and `bun run firefox:typecheck`.
- [ ] `bun run check:fix`, commit: `feat: auto-wait and actionability checks for interaction actions`.

---

### Task 1.4: Documentation

**Files:**
- Modify: `skills/htrcli/SKILL.md` (core-loop section: note that interaction commands now wait up to 5s for the element to be actionable, and that `--timeout`/options.timeout tunes it)
- Modify: `htcli/README.md` (Interaction section: same one-paragraph note)
- Modify: `CLAUDE.md` only if it makes claims the change invalidates (check; likely none)

**Steps:**

- [ ] Update both docs; re-read for contradictions with the new behavior (e.g. any advice telling users to sleep before clicking is now obsolete — remove or soften it).
- [ ] Commit: `docs: document interaction auto-wait behavior`.

---

## Manual verification (end of part)

Rebuild (`bun run build`), reload the extension in Chrome, then against a real page via htcli: click an element inside a modal that animates in — command should succeed without a manual sleep; click a bogus selector — should fail after ~5s with the "not found" message, not instantly.
