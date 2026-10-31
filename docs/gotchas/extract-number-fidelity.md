---
type: Gotcha
title: extract table/list textContent is lossy for numbers (footnotes, flattened superscripts)
description: 'extract table/list textContent is lossy for numbers: footnote markers and zero-width joiners stay in cells, and <sup> exponents flatten (e.g. 6.022×10^23 → 6.022×1023). Verified 2026-10-28 on Wikipedia infoboxes.'
resource: docs/benchmarks/office-task-benchmark.md §B14 (live run, 2026-10-28); scenario S16 (offline fixture proxy)
tags:
  - gotcha
  - extract
  - numbers
  - fidelity
  - citation-audit
  - benchmark
timestamp: 2026-10-31T09:07:25Z
---
# extract table/list textContent is lossy for numbers

`htrcli --cdp extract table "table.infobox" --json` (and `extract list`) returns each cell's `textContent`. The DOM text is real, but it is not number-safe: two lossy transformations happen silently.

## Trap 1 — footnote markers and zero-width joiners stay in the cell

The speed-of-light cell came back as `"299792458\u200d[2]"`: a U+200D zero-width joiner immediately before the `[2]` citation marker. A summary that quotes the raw cell imports a footnote ID into the number.

Fix: strip before citing:

```python
re.sub(r"\u200d?\[[^\]]*\]", "", value)
```

## Trap 2 — superscript exponents are flattened

`6.02214076×10<sup>23</sup>` reads back as `"6.02214076×1023"`. The mantissa survives, the exponent is gone, so the raw cell states 6.02×10^1023 — wrong by a thousand orders of magnitude. The `×` (U+00D7) and `−` (U+2212) also survive as typographic characters, not ASCII.

Fix: recover the exponent from the DOM (read the `<sup>` element or `innerHTML`), never from the flattened text.

## Neither is an extraction bug

The DOM text really is that. They are content traps for any workflow that cites numbers, so they belong in a gotcha rather than a defect list. `TODO.md` carries a "DECISION NEEDED" entry about whether extraction should render `<sup>` as `^` and strip footnote anchors; the behaviour was deliberately NOT changed.

## Citation audit recipe

The only automatable check for "no hallucinated facts" in a browser agent:

For each claim, re-read the source page in page context and test the claim:

```
htrcli --cdp eval "document.querySelector('#mw-content-text').innerText.includes('299792458')"  # → true
```

Critically, test a PERTURBED value too (`299792459`) — it must return `false`. Without the negative case the audit is vacuous, because it would agree with anything.

What this cannot prove: that the composed prose reasons correctly. The honest pass condition is "every number and reference traces to a live page", never "the text is true".

Context: live run recorded in `docs/benchmarks/office-task-benchmark.md` §B14 "Live run (2026-10-28)"; offline fixture proxy is scenario S16.