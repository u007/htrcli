---
type: Gotcha
title: CDP Drag Element-Ref Coordinate Resolution
description: CDP drag @eN refs resolve through the command-layer RefStore and convert backend nodes to viewport coordinates.
resource: htrcli/internal/commands/cdp_exec.go; htrcli/internal/commands/refstore.go; htrcli/internal/cdp/elementref.go
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
3. The CDP layer resolves the backend node with `DOM.getBoxModel`.
4. The content quad returned by `DOM.getBoxModel` is averaged to obtain the element center in document coordinates.
5. `Page.getLayoutMetrics` supplies the current visual viewport page offset. Subtract that offset to convert the center to viewport CSS coordinates for `Input.dispatchMouseEvent`.
6. Protocol failures, invalid quads, invalid backend node IDs, and non-finite coordinates remain explicit errors, including the affected ref/backend node where available.

This separation keeps persistence and `@eN` semantics in `htrcli/internal/commands`, while `htrcli/internal/cdp` owns CDP calls and coordinate conversion. The mapping is persisted in `~/.htrcli/refs.json`; refs are document-scoped and can become stale after navigation or detachment.

Relevant implementation: `htrcli/internal/commands/cdp_exec.go`, `htrcli/internal/commands/refstore.go`, and `htrcli/internal/cdp/elementref.go`.
