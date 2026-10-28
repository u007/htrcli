# Part 04 — `cdp <method> [params-json]` raw passthrough

## Problem

`htrcli command <json>` sends htrcli's own extension action JSON and is
rejected over `--cdp`. There is no way to issue an arbitrary DevTools Protocol
method for one-off inspection or a method htrcli has no verb for.

Session model to respect: every `--cdp` invocation opens a fresh session via
`cdpSession` in `internal/commands/cdp_exec.go` and closes it on return.
Session-scoped state set through this verb (anything under `Emulation.`,
`Network.setUserAgentOverride`, `Network.setExtraHTTPHeaders`, `Fetch.enable`,
and similar) therefore evaporates when the command exits. That is a documented
property, not a bug to work around.

## Files

- New `internal/commands/cdp_raw.go` — the verb.
- New `internal/commands/cdp_raw_test.go`.
- `internal/commands/inspect.go` ~line 484 — the `command` verb's help text
  should point CDP users to the new verb; leave its behaviour unchanged.
- `internal/cdp/session.go` — `Session.Call(method, params, result)` is the
  only thing needed.
- `README.md` ("Inspection" and "CDP transport" sections),
  `skills/htrcli/SKILL.md` (works/not-available table ~line 52 and a short
  note next to `command`), `TODO.md`.

## Design

- Usage: `htrcli --cdp cdp <Domain.method> [params-json]`. The params argument
  is optional; when present it must parse as a JSON object, otherwise error.
- CDP-only: on the extension transport return `errCDPOnly("cdp")`.
- `--frame` is rejected (raw calls are not scoped to a frame).
- Opens `cdpSession`, calls `Session.Call` with the raw params, and prints the
  raw result JSON (pretty-printed in text mode, as-is in `--json`). A CDP
  error becomes a non-zero exit with the protocol error text.
- When the method begins with `Emulation.`, print one stderr line saying the
  override lasts only for this command and that `emulate …` is the persistent
  verb (if the `emulate` verb group exists in the tree at implementation
  time; otherwise say the override is per-command only). No other method
  filtering; this is an escape hatch.

## Tests

- Argument validation: missing method, bad JSON, non-object JSON.
- Ext transport rejection and `--frame` rejection.
- Method-prefix check for the stderr note as a pure function.

## Docs

- SKILL.md: add `cdp` to the "Works" column; add a two-line note under the
  `command` description that `command` is the extension action JSON and `cdp`
  is the raw DevTools method, and that raw calls are stateless per invocation.
- README: same, with one example (`Browser.getVersion` or
  `Page.getLayoutMetrics`).
- TODO.md: note that stateful raw CDP (keeping a session open across
  invocations) is out of scope.
