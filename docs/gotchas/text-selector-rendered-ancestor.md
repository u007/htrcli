---
type: Gotcha
title: text= narrowing must fall back to the nearest rendered ancestor
description: A `text=` selector's innermost match can be an SVG title or a display:none duplicate with no layout box; findElement walks up to the nearest rendered ancestor, and keeps the innermost only when the whole subtree is hidden so `wait` can keep polling.
resource: src/contentScript/elementFinder.ts; src/contentScript/elementFinder.test.ts
tags: [content-script, selector, text-match, element-finder, svg, wait]
timestamp: 2026-11-19T03:54:00Z
status: active
---

# text= narrowing and rendered ancestors

`findElement({ text })` matches by substring, so `<html>`, `<body>`, the list and the row all match the label. `innermostMatches` drops any match that has a matching ancestor, leaving the element that carries the label; text split across children (`Mo<span>ve</span>`) still resolves to the container because no single descendant matches alone.

That innermost element is not always actionable:

- An SVG `<title>` carries the visible text but has no layout box.
- A page can keep a `display:none` duplicate of a visible label, and the duplicate wins document order.

So `findElement` maps results through `nearestRendered` (`src/contentScript/elementFinder.ts`). "Rendered" means `display`/`visibility` allow it and `getBoundingClientRect()` is non-zero; viewport position is deliberately ignored, because an off-screen label is still a valid target once scrolled into view.

- The walk uses `parentElement` and stops before `body`/`documentElement`. It returns the nearest ancestor with a box, so `click "text=Save"` hits the `<button>` rather than its hidden inner span.
- If **nothing** in the chain is rendered (a menu that is still closed), it returns the innermost match. Returning `<html>` there would make `wait "text=Organise"` resolve immediately against a huge element instead of polling until the menu opens.

Tests: `src/contentScript/elementFinder.test.ts` — "falls back to the nearest rendered ancestor when the innermost has no box" (`display:none` duplicate) and "keeps the innermost match when nothing in the subtree is rendered".

Do not "simplify" this to a rendered-only filter: dropping non-rendered matches would break `wait` on a closed menu, and returning the first rendered ancestor unconditionally would resolve to `body`.
