---
type: Gotcha
title: CDP Input Preparation Must Narrow Optional Coordinates
description: Narrow optional prepared CDP coordinates before assigning them to required numeric types.
resource: src/background/cdpInput.ts
tags:
  - CDP
  - TypeScript
  - input
  - coordinates
  - strict-mode
timestamp: 2026-08-24T09:10:41Z
---
## Gotcha

The CDP input preparation boundary returns optional coordinates because preparation can report focus-only state or fail to provide a viewport point. `PrepareSender` in `src/background/cdpInput.ts` therefore exposes `x?: number` and `y?: number`, while CDP dispatch payloads require numeric `x` and `y`.

Every caller that assigns prepared coordinates to a required coordinate type must narrow both values first:

```ts
if (typeof coords.x !== "number" || typeof coords.y !== "number") {
	throw new Error("prepareClick did not return viewport coordinates");
}
const point: { x: number; y: number } = { x: coords.x, y: coords.y };
```

Do not rely on the preparation action or a truthiness check to satisfy TypeScript. Validate each coordinate with `typeof ... === "number"` before constructing CDP `Input.dispatchMouseEvent` parameters or returning a required coordinate object. Without this narrowing, strict extension type-checks fail because optional `number | undefined` values are assigned to required numeric coordinates. The existing `dispatchCdpClick` and `resolveCoords` checks are the canonical pattern.