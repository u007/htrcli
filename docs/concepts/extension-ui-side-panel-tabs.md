---
type: Concept
title: 'Extension UI: toolbar icon toggles the side panel (no popup), two tabs'
description: Why the toolbar popup was removed, how Chrome and Firefox each turn a toolbar-icon click into a panel toggle, the two-view side panel (Recording/Tabs) with persisted roving-tabindex ARIA tabs, and the one manual release check.
resource: src/manifest.ts; firefox/vite.config.ts; src/background/index.ts; firefox/src/firefox-shims.ts; firefox/src/sidebarToggle.ts; src/sidepanel/SidePanel.tsx; src/sidepanel/tabState.ts; src/sidepanel/components/ConnectedTabs.tsx
tags:
  - extension
  - chrome
  - firefox
  - side-panel
  - ui
  - manifest
  - recording
timestamp: 2026-11-03T05:07:53Z
---
# Extension UI: toolbar icon toggles the side panel (no popup), two tabs

## Why this shape

The extension had a toolbar **popup** (`src/popup/*`, `popup.html`,
`firefox/popup.html`, `firefox/src/popup-entry.ts`) separate from the side
panel. Two UIs over the same recording state meant a user could start a
recording in the popup and have to open the panel to see the timeline, while the
connected-tab list lived somewhere else again.

The popup is gone. There is **one surface**: the Chrome side panel / Firefox
sidebar. A toolbar-icon click toggles it, so the tab list (the extension's main
reason to be one click away) is exactly one click from the toolbar.

Consequences you must preserve:

- No `action.default_popup` in either manifest. **A popup suppresses
  `action.onClicked`**, so re-adding one silently breaks the Firefox toggle.
  Chrome manifest (`src/manifest.ts`) and the Firefox generator
  (`firefox/vite.config.ts`) both carry a comment saying so.
- `src/popup/` no longer exists; the Firefox Vite input and the
  `firefox/tsconfig.json` `popup.html` entry were removed with it.

## Two-browser toggle contract

The manifests are now identical in intent but the toggle is wired per browser.

| Browser | Mechanism | Where |
|---|---|---|
| Chrome | `chrome.sidePanel.setPanelBehavior({openPanelOnActionClick: true})` — Chrome itself toggles the panel on an action click | `src/background/index.ts` |
| Firefox | `action.onClicked` → `sidebarAction.toggle()` | `firefox/src/sidebarToggle.ts`, registered from `firefox/src/firefox-shims.ts` |

Chrome's is documented as a toggle: the `PanelBehavior` reference says clicking
the icon toggles the panel. The service worker was verified to report
`getPanelBehavior()` → `{openPanelOnActionClick: true}`. A second click closing
the panel has **not** been exercised in a real browser; that is the manual check
listed under "Tests and the one manual gap".

Firefox has no equivalent runtime flag, so the Firefox shim installs the
listener itself:

- `registerSidebarToggle(browserApi)` returns `true` when the listener is
  installed. A missing `action.onClicked` or `sidebarAction.toggle` is
  **logged, not thrown** — recording and remote control must still work on a
  browser missing a UI-only capability.
- Registration is **idempotent per event object**: a second call on the same
  `action.onClicked` returns `true` without adding a second listener, because two
  listeners would turn one click into open-then-close.
- `sidebarAction.toggle()` is only legal inside a user-gesture handler, so it is
  called **synchronously** from the listener (no `await` before it), and as a
  **method** (`this` is the `sidebarAction` object). The returned promise only
  gets a `.catch` for logging.
- Registration happens **at import time** of `firefox-shims.ts`, which is
  imported only by `firefox/src/background-entry.ts` — never from the
  sidepanel/options entries, because every context that adds an
  `action.onClicked` listener is another candidate to run the toggle.
- Deliberately placed in `firefox/src/sidebarToggle.ts` rather than inline in
  the shim: the shim side-effect-imports `webextension-polyfill` and cannot load
  in a plain unit test, so the contract lives in a testable module
  (`sidebarToggle.test.ts`). It is also registered at import time rather than
  from the `chrome.sidePanel` stub, so a future Firefox that ships a real
  `sidePanel` cannot cause the toggle to be skipped.

## The side panel: two views

`src/sidepanel/SidePanel.tsx` exposes two ARIA tabs — **Recording** (default) and
**Tabs** — with `role="tablist"`/`role="tab"`/`role="tabpanel"` and
`aria-selected`/`aria-controls` wired up.

Accessibility and state rules that are load-bearing:

- **Roving tabindex**: exactly one tab has `tabIndex=0`. ArrowLeft/ArrowRight
  wrap, Home/End jump to the ends (all in `nextSidePanelTab`). Unrelated keys
  return `null` so typing is never hijacked.
- **Both panels stay mounted** (`hidden={activeTab !== …}`). Unmounting the
  Recording view would drop recording state, and unmounting the Tabs view would
  reset the tab-registry polling.
- The Tabs label carries a live **connected-tab count** badge, computed from the
  list `useConnectedTabs()` returns in the panel root.

### Recording view

`RecordingHeader` (start/stop, optional title, audio toggle) → `Timeline` →
`RecentRecordings` → `ExportPanel`.

`RecentRecordings` was moved from the popup: last 5 sessions, with delete routed
through the background `DELETE_SESSION` message (never IndexedDB directly) so
the live-session guard applies. `RecordingContext` refreshes the stored-session
list on `RECORDING_STOPPED` and `DELETE_SESSION_RESULT`. A real deletion is
broadcast, so the panel does not refresh again after it. The not-found reply
(`deleted: false`) is sent only back to the caller, so `RecentRecordings`
refreshes itself in that case. A delete error is shown only while its row is
still listed.

### Tabs view

`useConnectedTabs()` (in `ConnectedTabs.tsx`) is called once in the panel root.
It owns the connected-tab list, syncs on mount and polls every 3s. The Tabs
view and the tab-bar count badge both read that one list, so there is no
mirrored count state. `ConnectedTabs` takes the list as props and always renders
the same layout: the list fills the panel height and scrolls inside it.

## Persistence and robustness

The last view is remembered under the localStorage key
`htr.sidepanel.tab` (`src/sidepanel/tabState.ts`), so reopening the panel returns
the user to where they were instead of always resetting to Recording.

Two deliberate defences, both pinned by `tabState.test.ts`:

- `parseSidePanelTab` maps **anything** that is not `"tabs"` to `"recording"`, so
  a corrupt or older stored value is harmless.
- `globalThis.localStorage` can **throw on the getter** (not just return `null`)
  in a document with a denied storage partition. The storage is resolved
  **inside** the single `try` in `readSidePanelTab` / `writeSidePanelTab`, not in
  a default parameter, so the catch always runs. Each failure logs a warn and the
  panel degrades to "default view, keep working".

## Tests and the one manual gap

Automated:

- `src/sidepanel/SidePanel.test.tsx` — SSR structure: both tabs render, Recording
  is selected by default, a remembered `tabs` value is restored, an unknown
  value falls back to Recording.
- `src/sidepanel/tabState.test.ts` — parsing, storage read/write failure, a
  throwing `localStorage` getter, and keyboard navigation/wraparound.
- `src/sidepanel/components/ConnectedTabs.test.tsx` — rows come from the shared
  `tabs` prop, and there is no collapsible header.
- `firefox/src/sidebarToggle.test.ts` — listener registered, `toggle()` called
  synchronously and as a method on `sidebarAction`, double registration leaves one
  listener, and missing-API paths return `false` with a log.
- `bun run check`, tsc (root + Firefox), `bun test`, and both builds pass.
- A live headless-Chromium CDP check exercised tab switching and the persisted
  localStorage value against the built extension.

**Not automated:** the real toolbar-icon gesture. Chrome 156 ignores
`--load-extension` and the extension UI is browser chrome, so an actual icon
click cannot be scripted. The Firefox 159 icon click (load `firefox/build`,
click twice, confirm the sidebar opens then closes) is a **manual release
check**, recorded in `TODO.md` under "Extension side panel — unautomated
coverage". The same section notes that Firefox's own `sidebar_action` launcher
button is separate from the extension's `action` button and should be checked
for a double-toggle.

## Related

- [Driving htrcli from an agent host](../agent-host.md) — the extension as a
  command transport; the side panel is the human-facing surface over the same
  background worker.
- `README.md` (Usage step 1, and the "No toolbar popup" note in the extension
  section) and `CLAUDE.md` (`src/sidepanel/` architecture entry) describe the
  user-visible behaviour.
