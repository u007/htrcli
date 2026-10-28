# Part 03 — CDP parity: scroll, back/forward, printpdf, dialog handle, screenshot --annotate

## Problem

Six verbs are rejected over `--cdp` with `errUnsupportedCDP` although each is
one or two CDP calls. In headless CI the missing dialog handling is the worst:
a page that calls `confirm()` blocks the run with no remedy.

Rejection sites today:
- `internal/commands/interact.go` ~line 195 (`scroll`)
- `internal/commands/navigate.go` ~lines 51 and 84 (`back`, `forward`)
- `internal/commands/inspect.go` ~line 602 (`printpdf`)
- `internal/commands/dialog.go` ~lines 78 and 117 (`dialog handle`,
  `dialog list`)
- `internal/commands/cdp_exec.go` ~line 324 (`screenshot --annotate`)

Session model to respect: every `--cdp` invocation opens a fresh session via
`cdpSession` in `cdp_exec.go` and closes it on return. There is no daemon
buffer; `network wait --cdp` (`events_cdp.go`, `streamNetworkCDP`) is the
existing pattern for "attach, wait for an event for the command's duration,
exit".

## Files

- `internal/cdp/nav.go` (`Navigate`, `Screenshot`, `Reload`) — add history
  navigation helpers.
- `internal/cdp/screenshot.go` — add the annotate overlay helpers.
- `internal/cdp/dom.go` (`Evaluate`) and `internal/cdp/elementref.go` /
  `internal/cdp/input.go` (how a `TargetSelector` becomes an element handle;
  reuse that resolver for annotate).
- `internal/cdp/events.go` — pattern for enabling a domain and waiting for an
  event with a deadline.
- `internal/commands/cdp_exec.go` — `runScreenshotCDP`, new `runScrollCDP`,
  `runHistoryCDP`, `runPrintPDFCDP`, `runDialogHandleCDP`.
- The five command files listed above — replace the rejection with a call to
  the new CDP runner.
- Tests: `internal/cdp/nav_reload_test.go` and `screenshot_test.go` show the
  existing test style; `internal/commands/dialog_test.go` for flag parsing.
- `README.md`, `skills/htrcli/SKILL.md` (works/not-available table ~line 52
  plus the dialog section), `TODO.md` ("dialog handling" block).

## Design

### scroll

Evaluate `window.scrollBy` with the direction and pixel count mapped exactly
as the extension's `scrollTo` action does (default 500 px), then read back
`scrollX`/`scrollY` and report them. Not frame-aware (no annotation), same as
today.

### back / forward

`Page.getNavigationHistory`, pick `currentIndex ± 1`; if out of range, error
"no history entry to go back/forward to". Then `Page.navigateToHistoryEntry`
and wait for the load the same way `Navigate` does (reuse its wait, do not
duplicate it).

### printpdf

`Page.printToPDF` with `printBackground: true`, base64-decode, write the file,
print the same success line as the extension path. Errors from CDP propagate.

### dialog handle

Over CDP a policy cannot be armed in a daemon, so `dialog handle --cdp` is a
foreground waiter: enable `Page`, wait for `Page.javascriptDialogOpening`
until `--timeout` (new flag, ms, default 10000, same semantics as
`network wait`), answer it with `Page.handleJavaScriptDialog` using the
`--action` / `--text` already parsed by `parseDialogAction`, print the dialog
(type, message, default prompt text, what was answered) in text and `--json`,
then exit 0. Timeout with no dialog exits non-zero with a clear message. The
documented usage is the `network wait` pattern: start it in the background
before the click that triggers the dialog. `dialog list` stays unsupported
over CDP (no buffer); keep its rejection and say why in the error text.

The `--timeout` flag is only meaningful over CDP; on the extension transport
reject it if set, so behaviour never differs silently.

### screenshot --annotate

Resolve each `TargetSelector` from `parseAnnotateSelectors` to a live element
handle using the same resolver `cdp.Click` uses (so CSS, `text=`, `name=`,
`label=` and `@eN` all work). Through `Runtime.callFunctionOn` on each
element, inject a fixed-position overlay box with a numbered badge, using a
unique data attribute so cleanup can find them. Capture with the existing
`Screenshot` / `ScreenshotFullPage`. Remove every overlay afterwards, also on
the error path (defer). A selector that matches nothing is an error that names
the selector, after cleanup. Mirror the extension's overlay look only loosely;
numbering order equals argument order.

## Tests

- History index arithmetic and out-of-range errors as a pure function.
- Dialog: `--timeout` rejected on ext transport; timeout error text; the
  response struct shape in `--json`.
- Annotate: the injected overlay script is built by a pure function; test that
  it carries the index and the marker attribute, and that cleanup targets the
  marker.
- Existing tests keep passing.

## Docs

- SKILL.md table: move `scroll`, `back/forward`, `printpdf`,
  `screenshot --annotate`, `dialog handle` to the "Works" column, leaving
  `dialog list` (with the reason) in the other column. Add the background-wait
  recipe for `dialog handle --cdp`.
- README: same in the dialog section and the CDP transport section.
- TODO.md: record that `dialog list` and persistent dialog policies are not
  available over CDP because there is no long-lived session.
