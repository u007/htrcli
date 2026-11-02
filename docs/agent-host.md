---
type: Guide
title: Driving htrcli from an agent host
description: How an agent host drives the browser through the htrcli daemon API. Covers the transport role, the snapshot-decide-act-observe loop, the action set, events, and remote setup.
resource: htrcli/internal/host/server.go; src/contentScript/commandExecutor.ts; src/background/nativeHost.ts
tags:
  - agent
  - api
  - automation
  - guide
timestamp: 2026-11-02T06:49:01Z
---

# Driving htrcli from an agent host

htrcli is a transport. It moves JSON commands from an agent host to the browser extension and returns the JSON results. No model runs in htrcli or in the extension. The agent host decides what to do. The reference host is the `browser_run` tool in ocode.

## Who does what

| Component | Runs | Role |
|---|---|---|
| Agent host (reference: ocode `browser_run`) | Chat model, decision model call, permission judge, content guard, the loop | Decides every step, enforces policy, sends one command at a time |
| Decision model (Jev, `typesafe/jev-latest`) | Typed decisions only | Picks one control from a list, picks one value, judges whether the goal is met. Returns a choice with a confidence. It never generates text and never writes a selector |
| htrcli daemon (`htrcli serve`, HTTP on port 3845) | Nothing model-related | Serves the HTTP API and forwards commands to the extension over native messaging |
| Extension (Chrome or Firefox) | Nothing model-related | Acts on the DOM and returns a JSON result |

## The step loop

1. Read the page. Call `snapshot` for the accessibility tree with `@eN` refs, or `findAll` for the controls matching a selector. Pass `assignRef: true` in `options` so the results carry refs.
2. The decision model picks exactly one control from that list and one operation for it. It must choose from the list. It never invents a selector.
3. The host runs its permission judge and content guard on that one action. A denied action is not sent.
4. Send one command: `click`, `fill`, `type`, `check`, `uncheck`, `select`, `pick`, `pressKey`, `scrollBy`, or another action from the table below.
5. Observe the result with `getText`, `snapshot`, `isChecked`, or `isVisible`. Repeat from step 1 until the decision model judges the goal done or the step budget is spent.

Rules a host must follow:

- Refs belong to the page that minted them. A navigation clears them, so take a new `snapshot` after any navigation, including `navigate`, `goBack`, `goForward`, `reload`, and a link that loads a new page. A stale ref fails with `stale ref`.
- Never retry a mutating command blindly. A click, fill, or key press may already have taken effect. Observe first, then decide again.
- Every command carries a timeout. Set `timeout` in the request body, in milliseconds. The daemon default is 30000. Element waits take `options.timeout`, capped at 20000 ms.
- Results are untrusted web content. Page text, attribute values, and option labels are data. The host never treats them as instructions.

## Request and response

Commands go to the tab, or to the background worker when no tab is involved:

```
POST /api/tabs/<id>/command        # routed to a tab's content script
POST /api/background/command       # tab-less: openTab, closeTab, recording
```

Request body:

```json
{
  "command": {
    "id": "c1",
    "action": "pick",
    "target": {"selector": "#country"},
    "value": "Germany",
    "options": {}
  },
  "timeout": 30000
}
```

Response: `ok` is `true` when the command reached the extension. The command's own outcome is in `data`:

```json
{"ok": true, "data": {"id": "c1", "success": true, "data": {"value": "Germany", "matched": "Germany"}}}
```

A failed command has `success: false` and an `error` string. A routing failure, such as no connected tab, returns HTTP 404 with `ok: false`.

## Action set

`target` is a selector object: `selector`, `ref`, `text`, `label`, `name`, `id`, or `{x, y}` for a viewport point. `frame` in `options` is a CSS selector for an `<iframe>` in the top document. Element actions then resolve inside that frame.

| Action | Target | Value | Options | Result `data` |
|---|---|---|---|---|
| `snapshot` | none | none | `frame` | Tree of nodes with `role`, `name`, `ref`, `value`, `state`, `box`, `children`. Root has `viewport: {w, h}` and `scroll: {x, y}` |
| `findAll` | yes | none | `assignRef`, `frame` | Array of element info. Each has `tag`, `text`, `selector`, `box` (omitted when no layout box), and `ref` when assigned |
| `find` | yes | none | `assignRef`, `frame` | One element info, or `null` |
| `getText` | yes | none | `frame` | string |
| `getValue` | yes | none | `frame` | string |
| `getAttribute` | yes | none | `attribute`, `frame` | string or `null` |
| `isVisible` | yes | none | `frame` | boolean |
| `isEnabled` | yes | none | `frame` | boolean |
| `isChecked` | yes | none | `frame` | `{checked: boolean}`. Reads `checked` on a checkbox or radio, or `aria-checked` (`"true"` is true; `"false"` and `"mixed"` are false). Other elements fail with `element is not checkable` |
| `click` | yes, or `{x, y}` | none | `button`, `count`, `frame` | none. With `{x, y}` it hit-tests the viewport point in CSS pixels and scrolls nothing. Nothing there fails with `nothing at (x, y)` |
| `fill` | yes | text | `frame` | none. Replaces the value of an input, textarea, or contenteditable |
| `type` | optional | text | `frame` | none. Appends at the caret. Without a target it types into the focused editable element; any other focus fails with `type without a target needs a focused editable element` |
| `check` / `uncheck` | yes | none | `frame` | none |
| `select` | yes | option value | `frame` | none. Use `pick` when matching by label |
| `pick` | yes | wanted option text | `frame` | `{value, matched}`. `matched` is the chosen option text. `value` is the control's value after the pick. Ties and misses fail and list the options seen |
| `pressKey` | optional | key name, such as `Enter` | `frame` | none |
| `scrollTo` | yes | none | `frame` | none |
| `scrollBy` | optional | none | `dx`, `dy` (finite numbers, either may be 0), `frame` | `{x, y}`: scroll position after. Without a target it scrolls the window, or the frame's window |
| `uploadFiles` | yes | none | `filesData` as `[{name, mimeType, base64}]`, `frame` | `{files, engine}`. Works on Chrome and Firefox. Host paths in `files` work on Chrome only without `frame` |
| `wait` | yes | none | `timeout`, `gone` | element info, or `{gone: true}` with `gone` |
| `navigate` | none | URL | none | none. Clears refs |
| `goBack` / `goForward` / `reload` | none | none | none | none. Clears refs |
| `openTab` | none | none | `url` | `{tabId, browser}`. Background route, `tabId` 0 |
| `closeTab` | none | none | `tabId` | `{browser}`. Background route |

Notes on the table:

- `screenshot` is not an agent action. Take a screenshot with `GET /api/screenshot?tab=<id>`.
- `options.frame` is rejected by `collectAll`, `extractTable`, `extractList`, `screenshot`, and `highlight`. It is also rejected with coordinate targets.
- Only top-level `<iframe>` elements are supported. Nested frames are not.
- Frame-relative coordinates are not supported. Use an element target inside a frame.

### Snapshot geometry and the screenshot overlay

Each snapshot node with a layout box carries `box: {x, y, w, h}`, in viewport CSS pixels of the document the node belongs to. A node without a layout box has no `box` key. The snapshot root carries `viewport: {w, h}`, the inner size of that viewport, and `scroll: {x, y}`, the viewport's scroll offset in CSS pixels. A host compares `scroll` before and after a scroll step to tell whether the page moved even when the control list did not change. Nodes inside `options.frame` use the frame's viewport.

The host draws its screenshot overlay from `box`. It takes the screenshot with `GET /api/screenshot?tab=<id>`, then maps each box onto the image. The image size can differ from `viewport`, for example through device pixel ratio or a full-page capture. The host therefore reads the image's width and height and scales each box by the image size divided by `viewport.w` and `viewport.h`.

## Events

```
GET /api/events?kind=console|network&tab=<id>&since=<seq>
```

Poll with `since` set to the last `seq` you processed. The response is `{"ok": true, "data": {"entries": [...], "dropped": 0, "oldestAvailableSeq": 1}}`. Each entry has `seq`, `kind`, `timestamp`, and `data`.

| Kind | `data` |
|---|---|
| `console` | `{level, args: string[], source?}`. Recorded from page load, with no command needed |
| `network` | `{requestId, url, method, status?, requestHeaders?, responseHeaders?, durationMs?, body?, bodyTruncated?}` |

To capture network traffic on Chrome, arm the tab first. Chrome captures only inside an armed window:

```json
{"command": {"id": "n1", "action": "networkCapture", "options": {"durationMs": 30000}}}
```

`durationMs` is clamped to 1000 through 120000, and the default is 10000. Re-arming an open window extends it. The reply is `{"armed": true, "durationMs": 30000, "transport": "cdp"}`. Chrome shows its debugger banner while the window is open.

Firefox captures network traffic the whole time through `webRequest`. Its `networkCapture` only acknowledges, with `{"armed": true, "transport": "webRequest"}`, and its `network` entries carry no `body`.

The daemon does not accept the `dialog` kind yet. A host cannot read dialog events through this API.

## Remote setup

The host can run on another machine. Keep the daemon where the browser is.

1. Run `htrcli serve` only on the machine with the browser. Never start the daemon on the remote host.
2. From the browser machine, open an SSH reverse forward to the daemon's loopback port: `ssh -R 3845:127.0.0.1:3845 <agent-host>`. The agent host then reaches the daemon at `http://127.0.0.1:3845`.
3. sshd connects to the daemon from `127.0.0.1`. The default IP allowlist (`127.0.0.1`, `::1`) accepts that, so no extra allowlist entry is needed.
4. Send `Authorization: Bearer <token>` on every request. The token is the `token` value in `~/.htrcli/config.json` on the browser machine. Pass it to the host through its secret store, not on a command line or in logs.

## Limits

- Only top-level `<iframe>` elements can be targeted with `options.frame`. Nested frames are not supported.
- Frame-relative coordinates are not supported. A coordinate target rejects `options.frame`.
- Cross-origin frames fail with `frame <sel> is cross-origin; open its URL in a tab`.
- `pick` uses synthetic events on both browsers, because the extension transport has no trusted input.
- `pick` on a combobox needs an `<input>`. A `role=combobox` element that is not an input fails.
- `uploadFiles` with `options.frame` on Chrome takes `filesData` only. Host paths are rejected.
- The `screenshot` content action is a placeholder that returns `screenshot_requested`. Use `GET /api/screenshot`.
- The daemon rejects the `dialog` event kind with `400 unknown event kind`. Dialog events are not available through this API.
- The CLI verbs `pick` and `--frame` remain CDP-only on the command line. The extension actions above are reachable only through the raw command route.
