# Part 02 — `emulate` verb group (viewport, color scheme, user agent)

## Problem

htrcli has no device-metrics or media emulation. Testing a 390 px layout or
dark mode today means restarting Chrome with window flags, which does not even
give the requested viewport in headless mode. CDP has
`Emulation.setDeviceMetricsOverride`, `Emulation.setEmulatedMedia`, and
`Network.setUserAgentOverride`, but all three are session-scoped: they reset
when the DevTools session detaches. Every `htrcli --cdp` command opens a fresh
session in `cdpSession` and closes it on return, so a one-off call is useless.
The fix is persisted emulation state that `cdpSession` re-applies on every
open.

## Files

- New `internal/cdp/emulation.go` — the `Emulation` struct, `ApplyEmulation`,
  `ClearEmulation`.
- New `internal/cdp/emulation_test.go`.
- New `internal/commands/emulate.go` — cobra verbs and the persisted store.
- New `internal/commands/emulate_test.go`.
- `internal/commands/cdp_exec.go` — `cdpSession` applies the stored emulation
  right after `PageSession` returns and `SetFrame` is set.
- `internal/commands/refstore.go` — the pattern to copy for a JSON store under
  `~/.htrcli` with a test-overridable path variable.
- `internal/commands/cdp_exec.go` `errCDPOnly` — use for the extension
  transport.
- `README.md` ("CDP transport" area, "Inspection" or a new "Emulation"
  section), `skills/htrcli/SKILL.md` (works/not-available table ~line 52 and a
  short "Emulation" section near the CDP transport section), `TODO.md` (new
  known-limitations block).

## Design

### Verbs (all CDP-only; extension transport returns `errCDPOnly`)

- `emulate viewport <width>x<height>` with `--scale <float>` (device scale
  factor, default 1) and `--mobile` (sets the mobile flag and touch
  emulation). Width and height are positive integers; parse errors are loud.
- `emulate color-scheme <light|dark|none>`; `none` removes the override.
- `emulate user-agent <string>`; empty string removes the override.
- `emulate show` prints the stored emulation for the current CDP port (text
  and `--json`).
- `emulate reset` removes everything for the current CDP port and issues the
  CDP clear calls once on a live session.

Each setting verb: parse, load the store, update the entry, save, then open a
session with `cdpSession` and apply once so invalid values fail at the command
rather than on the next unrelated verb. The response confirms what is now in
effect, read back through `window.innerWidth/innerHeight`,
`matchMedia('(prefers-color-scheme: dark)').matches`, and `navigator.userAgent`
via the existing `Evaluate` helper.

### Store

`~/.htrcli/emulation.json`, a JSON object keyed by CDP port (string), each
value an `Emulation` struct: width, height, scale, mobile, colorScheme,
userAgent. Keyed by port (whole browser), not by tab, because `--tab` is often
omitted on CDP and a layout test wants every tab emulated the same way. Path is
a package variable overridable in tests exactly like `refStorePath`. A missing
file means no emulation; a malformed file is an error.

### Apply on every session

`ApplyEmulation(s, e)` in `internal/cdp`:
- width or height non-zero → `Emulation.setDeviceMetricsOverride` with width,
  height, deviceScaleFactor, mobile; with `--mobile` also
  `Emulation.setTouchEmulationEnabled`.
- colorScheme non-empty → `Emulation.setEmulatedMedia` with a
  `prefers-color-scheme` feature.
- userAgent non-empty → `Network.setUserAgentOverride`.
Any CDP error is returned as-is; `cdpSession` wraps it with
"applying stored emulation for port N" and fails the command. Nothing is
skipped silently.

`ClearEmulation(s)` issues `Emulation.clearDeviceMetricsOverride`,
`Emulation.setEmulatedMedia` with no features, and
`Network.setUserAgentOverride` with an empty string.

`cdpSession` loads the store once per invocation. No emulation stored → no
CDP calls, so existing behaviour and timings are unchanged.

### Interaction notes to document

- `screenshot` and `page` reflect the emulated viewport on the next call.
- `tabs new` opens a tab that gets the emulation on its first htrcli command,
  not before; a page loaded in between may need a `reload`.
- Emulation is per CDP port; `browser stop` does not clear the store, so a
  restarted browser is emulated again. `emulate reset` clears it.

## Tests

- Store round trip, missing file, malformed file, multiple ports.
- Argument parsing for `WxH`, scale, color-scheme enum, rejection of bad input.
- `ApplyEmulation` against a fake `Session` transport if one exists in
  `session_test.go`; otherwise test the params builders as pure functions
  (method names and param maps) and keep the CDP call thin.
- `cdpSession` with an empty store makes no extra calls (assert via the same
  fake or by a counter on the params builder).

## Docs

- README and SKILL.md: new "Emulation (--cdp)" section with the verbs, the
  persistence model, and the three interaction notes above. Add `emulate` to
  the SKILL.md "Works" column.
- TODO.md: note that emulation is per port, not per tab, and that other
  emulations (locale, timezone, geolocation, network conditions) are not
  implemented.
