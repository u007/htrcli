---
type: Decision
title: htrcli transport choice — extension by default, `--cdp` only for a fresh session
description: User policy (2026-11-18) — drive the user's real browser through the extension unless the task needs a fresh, isolated or headless session; never use `--cdp` for a site behind a login. Lists the only legitimate CDP reasons, the human-signs-in-once profile pattern, and the CLI verbs that are CDP-only (`pick`, `fill-form`, `--frame`).
resource: htrcli/internal/commands/root.go; htrcli/internal/commands/pick.go; htrcli/internal/commands/fillform.go; htrcli/internal/cdp/launch.go; htrcli/internal/cdp/context.go; src/contentScript/commandExecutor.ts; src/manifest.ts
tags:
  - htrcli
  - transport
  - cdp
  - extension
  - policy
  - browser
timestamp: 2026-11-18T15:58:27Z
---

# htrcli transport choice

**Extension first. `--cdp` only when the task genuinely needs a fresh, isolated or headless session.** Every task that depends on an authenticated session goes through the extension, never CDP.

## The rule

- Plain `htrcli` (no flag) uses the extension. `--cdp` is shorthand for `--transport cdp`; `UseCDP()` resolves flags > config/env > `ext`, so the extension is the default (`htrcli/internal/commands/root.go`, the `--transport`/`--cdp` flag registration and `UseCDP`).
- Anything behind a login — Gmail, Slack, a CRM, an admin console, any page that shows a sign-in form — takes the extension transport with no flag.
- `--cdp` is reserved for the reasons below. A reason is a requirement of the task, not a preference; habit is not a reason.

## Why CDP starts logged out

`htrcli browser start` launches its own Chrome with `--user-data-dir` set to a dedicated directory:

- `~/.htrcli/chrome-profile` by default (`htrcli/internal/cdp/launch.go`, `ProfileDir`).
- `~/.htrcli/contexts/<name>` for a named `--context` (`htrcli/internal/cdp/context.go`, `ContextProfileDir`).

That profile is empty on first use: it has none of the cookies, SSO state or MFA-cleared sessions that exist in the user's normal browser. The extension, by contrast, is injected into the user's own `http`/`https` pages, so the session that is already authenticated is the one the agent drives.

## Never log in through CDP

A login wall, consent screen or CAPTCHA on CDP means the transport is wrong. It is a hard stop, not a step to work around: an agent must never enter credentials.

A CDP profile becomes authenticated only when a human signs in to that dedicated profile once — start it visibly with `htrcli browser start`, sign in by hand, then reuse the profile (including headless). That is the bench/CI pattern for a pinned scenario context. It is not a substitute for the user's own session, and it does not make `--cdp` the right transport for a logged-in task.

## Legitimate `--cdp` reasons (the whole list)

1. A clean profile with no cookies, history or user state.
2. Headless or CI/cron automation.
3. Pages the extension cannot reach, such as `chrome://` URLs or the Chrome Web Store developer console.
4. Local `file://` fixtures, because the content scripts match only `http://*/*` and `https://*/*` (`src/manifest.ts`, `firefox/vite.config.ts`).
5. A scratch `--context` that must not inherit the user's state. A named context is a fresh, isolated CDP profile — not a way to reach a logged-in site.

## CLI verbs that are CDP-only

These are rejected on the extension, so a task that needs one of them must use `--cdp`:

- `pick` — `errCDPOnly("pick")`, "pick is only supported over --cdp" (`htrcli/internal/commands/pick.go`).
- `fill-form` — `errCDPOnly("fill-form")` (`htrcli/internal/commands/fillform.go`).
- `--frame` — `validateFrameFlag(..., UseCDP())` rejects it on the extension with "--frame is only supported over --cdp" (`htrcli/internal/commands/root.go`).

This is a capability constraint, not permission to browse a logged-in site over CDP. The daemon's raw command route does expose an extension-side `pick` action (see `agent-host.md`), but the CLI verbs above remain CDP-only.

## Extension limits that change a plan — but not the transport

- `select` sets `<select>.value` only (`src/contentScript/commandExecutor.ts`, `handleSelect`). A label that is not an option value selects nothing, silently, with no error. The extension CLI has no `pick` verb, but the extension raw command route does expose the `pick` action (see `agent-host.md`); the CLI-side extension path is to click the input, type the query, click the `[role=option]` or `text=<option>`, then read the value back.
- `file://` pages have no content script (see reason 4), so they need CDP.

## Where this is also recorded for agents

`skills/htrcli/SKILL.md` carries the agent-facing form: a "Transport choice: extension first, `--cdp` only for a fresh session" section, extension-first quick rule 1, a fresh-session-only CDP section, the browser-contexts note (a context is a fresh CDP profile, not a route to a logged-in site), and the Office-apps recipes on the extension transport.

## Verification

This is a documentation and policy change; no test exercises it. The code facts above (default transport resolution, profile paths, `errCDPOnly` for `pick`/`fill-form`, `validateFrameFlag`, content-script match patterns, `handleSelect`) were read from the source on 2026-11-18.
