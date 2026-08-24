---
type: Gotcha
title: Firefox Coordinate Input Requires Hit-Testing
description: Firefox synthetic coordinate mouse and drag events need elementFromPoint hit-testing before dispatch.
resource: src/contentScript/commandExecutor.ts
tags:
  - Firefox
  - input
  - mouse
  - drag
  - hit-testing
timestamp: 2026-08-24T09:10:27Z
---
## Gotcha

Firefox does not perform viewport hit-testing for synthetic mouse or drag events when they are dispatched on `document.body`. Supplying `clientX`/`clientY` in the event initializer only sets coordinates on the event; it does not retarget the event to the element under those coordinates.

When a command accepts literal viewport coordinates, resolve the target first with `document.elementFromPoint(x, y)` (and validate the result) and dispatch the pointer/mouse events on that element. Apply the same rule to drag source, intermediate movement, and destination events as needed. Dispatching coordinate events on `document.body` can bypass the page control that should receive them, particularly in Firefox.

The coordinate-input paths are in `src/contentScript/commandExecutor.ts` (`handleMouseDown`, `handleMouseUp`, `handleMouseMove`, and `handleDrag`). Element-targeted commands should continue dispatching on the resolved element rather than using `document.body` as a substitute.