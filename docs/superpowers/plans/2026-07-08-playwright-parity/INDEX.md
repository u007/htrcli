# Playwright-Parity Implementation Plan — Index

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring HTR NControl's remote-control actions up to Playwright-grade reliability: auto-waiting, actionability checks, post-action navigation settling, trusted input, and correctness fixes for `wait`/`scrollTo`/`evaluate`.

**Architecture:** All four parts build on the navigation-wait work already merged (background intercepts nav actions and waits for `tabs.onUpdated status=complete`; the Bun server polls `pageInfo.readyState`). Part 1 adds waiting/actionability inside the content script's command executor. Part 2 extends the two transport layers (background native-host relay, Bun server) to detect and wait out navigations *triggered by* actions. Part 3 moves input synthesis to CDP in the background for trusted events on Chrome, keeping the synthetic path as the Firefox behavior. Part 4 is three small correctness fixes in the command executor.

**Tech Stack:** TypeScript (extension, Bun server), Biome (tabs, double quotes), Bun test runner (+ happy-dom for DOM tests, added in Part 1), Chrome extension MV3 APIs (`chrome.tabs`, `chrome.debugger`), webextension-polyfill for Firefox.

## Global Constraints

- Package manager: `bun` only — never npm/yarn.
- Lint/format: run `bun run check:fix` before every commit.
- All cross-component message types live in `src/types/recording.ts` / `src/types/commands.ts`; server mirrors in `server/types.ts` — keep the two `commands.ts`/`types.ts` copies byte-identical for shared interfaces.
- Extension error log prefix: `[HTR NControl]` (content/background), `[NativeHost]` (native-host module).
- Firefox shares 100% of `src/`; any Chrome-only API use must gate on availability (existing pattern: `typeof chrome.debugger === "undefined"`) and provide a Firefox path — never break `bun run firefox:typecheck`.
- Command round-trip ceilings: htcli HTTP client 30s, daemon command timeout 30s, server `DEFAULT_COMMAND_TIMEOUT` 30s. Any new internal wait must complete (or fail loudly) within 25s so callers get clean errors, matching the existing `NAV_LOAD_TIMEOUT_MS = 25000` convention.
- No silent fallbacks: every timeout or degraded path returns an explicit error message naming the action and what condition wasn't met.
- New dev dependencies must pin a major version (no `latest`).
- After each part: `bunx tsc --noEmit`, `bun test`, `bun run build`, `bun run firefox:typecheck` must all pass.

## Parts

| # | File | Scope | Risk |
|---|------|-------|------|
| 1 | `01-auto-wait-actionability.md` | Content script: default auto-wait + visible/enabled checks for all interaction actions; scroll-before-click | Medium — changes default behavior of every interaction action |
| 2 | `02-post-action-navigation-settling.md` | Background relay + Bun server: after click-class actions, detect a triggered navigation and wait for load | Medium — touches both transports |
| 3 | `03-trusted-input-via-cdp.md` | Background: CDP `Input.*` dispatch for click/pressKey/type on Chrome; synthetic pointer-event upgrade as shared fallback | High — debugger attach UX, per-browser divergence |
| 4 | `04-small-correctness-fixes.md` | Command executor: `wait` errors on timeout, `scrollTo` instant + settled, `evaluate` multi-statement + async | Low |

## Execution Order

1 → 4 → 2 → 3.

- Part 1 first: it introduces the `waitForActionableElement` helper and the happy-dom test harness that Parts 3 and 4 tests rely on.
- Part 4 second: small, independent, and its `wait`-action change should land before Part 2's settling logic so timeout semantics are consistent everywhere.
- Part 2 third: independent of Part 1 at the code level but easier to verify manually once actions stop failing instantly.
- Part 3 last: highest risk (debugger attach infobar, key-mapping table, Firefox divergence); everything else must be stable before it lands.

Each part produces working, independently shippable software; stop-and-review after every part.

## Out of Scope (explicitly)

- Read-only actions (`find`, `getText`, `getValue`, …) stay instant-fail — probing semantics are intentional; documented in Part 1.
- `openTab` load-waiting — separate follow-up if needed.
- Screenshot font/raf settling.
- Any htcli (Go) changes — all four parts are extension/server-side; the CLI wire format is unchanged.
