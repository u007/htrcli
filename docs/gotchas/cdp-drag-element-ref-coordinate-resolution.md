---
type: Gotcha
title: CDP Drag Element-Ref Coordinate Resolution
description: CDP drag @eN refs resolve through the command-layer RefStore and reach the page-side drag resolver as element handles, never as pre-computed pixels.
resource: htrcli/internal/commands/cdp_exec.go; htrcli/internal/commands/refstore.go; htrcli/internal/cdp/input.go
tags:
  - cdp
  - drag
  - element-refs
  - coordinates
  - gotcha
timestamp: 2026-08-24T09:36:19Z
---
# CDP drag element-ref coordinate resolution

CDP drag commands must resolve persistent `@eN` element references in the command layer before the drag protocol operation runs.

## Resolution flow

1. The command layer loads the persistent `RefStore` and looks up each `@eN` reference to obtain its CDP `backendNodeId`.
2. A missing ref-store entry is a stale-reference error and must be reported explicitly; do not silently fall back to selector resolution or coordinates.
3. The command layer passes the `backendNodeId` to `cdp.Drag` as a `DragEndpoint` — **not** a coordinate. A ref converted to pixels before the drag runs is stale by the time the other endpoint's `prepareClick` has scrolled the page.
4. The CDP layer turns the node into a live element handle (`DOM.resolveNode` → `objectId`) and invokes the page-side drag resolver through `Runtime.callFunctionOn` with that element as an argument, alongside any selector endpoint. The resolver scrolls BOTH endpoints into view first, then reads both centres; if one is outside the viewport it scrolls each endpoint's clipping overflow containers (the window cannot reach those), fit-scrolls the window once, and re-reads — the same discipline for refs, selectors and their mixes.
5. A node that no longer resolves to an element (`DOM.resolveNode` returns null) is an explicit stale-ref error naming the backend node.
6. `@eN` endpoints cannot be combined with `--frame` (refs live in the top document); the CDP layer rejects that combination explicitly.

This separation keeps persistence and `@eN` semantics in `htrcli/internal/commands`, while `htrcli/internal/cdp` owns the protocol calls. The mapping is persisted in `~/.htrcli/refs.json`; refs are document-scoped and can become stale after navigation or detachment.

Relevant implementation: `htrcli/internal/commands/cdp_exec.go`, `htrcli/internal/commands/refstore.go`, and `htrcli/internal/cdp/input.go` (`DragEndpoint`, `dragPointsFn`, `resolveDragPoints`).
