---
type: Gotcha
title: Drag Input Bounds
description: Established bounds and validation contract for drag steps, delays, and empty key specifications.
resource: htrcli/internal/cdp/input.go; htrcli/internal/commands/interact.go
tags:
  - drag
  - input
  - keyboard
  - bounds
  - gotcha
timestamp: 2026-08-24T09:36:30Z
---
# Drag input bounds

The drag-input contract normalizes caller options before dispatching mouse events:

- `steps` is constrained to `1..100`. A value below `1` uses the default of `5`; values above `100` are clamped to `100`.
- `delay` is expressed in milliseconds and constrained to `0..2000`. Negative values use `0`; values above `2000` are clamped to `2000`. The default is `0` ms.
- Key commands reject an empty or whitespace-only key specification with a clear error (`key cannot be empty`). A trailing `+` is also rejected because it leaves the key portion empty.

These bounds keep drag interpolation and per-move sleeps predictable, while preventing invalid key requests from reaching CDP. The normalization is implemented in `htrcli/internal/cdp/input.go` and the CLI defaults are declared in `htrcli/internal/commands/interact.go`.
