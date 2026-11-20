---
type: Unclassified
title: htrcli Office-Task Benchmark
description: 'htrcli office-task benchmark: the B-series task specs, the S-series offline fixtures, the bench harness (verdicts, judge, evidence, exclusions), the browser-use BU Bench V2.1 alignment (structure only, no imported task text), built offline proofs for B21–B24, a shape-only comparison with Browser Use''s published runs, and the judge validation record (stub-proven; measured with a local Qwen3-0.6B smoke-grade judge).'
tags:
  - benchmark
  - htrcli
  - browser-automation
  - judge
timestamp: 2026-11-15T23:05:21Z
---
# htrcli Office-Task Benchmark

Evaluation suite for htrcli (browser automation) on common cowork / office jobs.
Each scenario is a **task an agent must complete end-to-end using htrcli**
against a real web app. Scenarios are ordered by **priority (P0 first)**.
Success is measured by the listed pass/fail check, not by "the command ran".

## Safety (read first)

- Run against a **throwaway Google/test account** or draft-only data.
- Make every **send** and **book** step dry-run or self-addressed: reply to
  yourself, create a draft instead of sending, or book a clearly-labelled
  test slot. A benchmark run must never email real people or make real
  bookings.
- Never enter payment card details or solve CAPTCHAs.

## Baseline (non-browser)

The `gws-*` / Gmail MCP tools are a **non-browser baseline for comparison**,
not a replacement: record their success/time/command-count alongside htrcli
so regressions or wins are visible. htrcli is the browser path under test.

## Metrics (record per run)

- `success`: PASS / PARTIAL / FAIL (the harness also reports NOT_RUN, TIMEOUT
  and EXCLUDED; none of those is a PASS)
- `commands`: number of htrcli invocations
- `wall_ms`: wall-clock milliseconds
- `retries`: number of retries / recoveries
- `failure_mode`: short description if not PASS

Scoring: PASS = all pass/fail checks met on first attempt. PARTIAL = completed
with 1 retry/recovery. FAIL = not completed or >2 retries.

## Results (executed 2026-11-16)

Environment: htrcli built from this tree, Chrome 157 on CDP port 9555, all
fixtures served from `docs/benchmarks/fixtures/`, `--transport cdp`. No network
except the two live rows. The frozen summary of this run is
`docs/benchmarks/results-2026-11-16.json`; the per-target report JSONs land in
the gitignored `docs/benchmarks/results/`, so the frozen file is what is
committed. Cost is not in these tables at all — no driver supplied token usage,
and `htrcli` spends none.

One invocation produced every offline row below (`make …` once), so the rows
and the totals are consistent with each other rather than stitched from reruns.

Reproduce:

```bash
make htrcli-build
make bench-smoke bench-office bench-tasks bench-form bench-booking bench-expense \
     bench-files bench-mail bench-crm bench-messaging bench-tickets bench-leave \
     bench-social bench-survey bench-finance bench-store bench-research \
     bench-cart bench-retrieval bench-compare bench-challenge bench-slack
./htrcli/bin/htrcli bench run docs/benchmarks/scenarios/s29-b22-live.json \
  --transport cdp --allow-network
./htrcli/bin/htrcli bench run docs/benchmarks/scenarios/s31-b23-live.json \
  --transport cdp --allow-network
make bench-judge   # needs JUDGE_URL / JUDGE_MODEL / JUDGE_AUTH
```

### Offline suite — 29 rows, 27 PASS

| Scenario | Verdict | Commands | wall_ms |
|---|---|---|---|
| S01 | PASS | 7 | 273 |
| S02 | PASS | 5 | 133 |
| S03 | PASS | 5 | 227 |
| S04 | PASS | 7 | 350 |
| S05 | PASS | 5 | 175 |
| S06 | PASS | 5 | 183 |
| S07 | PASS | 9 | 264 |
| S08 | PASS | 11 | 270 |
| S09 | PASS | 14 | 273 |
| S10 | PASS | 17 | 408 |
| S11 | PASS | 22 | 612 |
| S12 | PASS | 17 | 576 |
| S13 | PASS | 22 | 724 |
| S14 | PASS | 39 | 983 |
| S15 | PASS | 38 | 1024 |
| S16 | PASS | 30 | 802 |
| S17 | PASS | 36 | 998 |
| S18 | PASS | 49 | 1096 |
| S19 | PASS | 73 | 1757 |
| S20 | PASS | 88 | 2806 |
| S21 | PASS | 81 | 1892 |
| S22 | PASS | 62 | 1400 |
| S23 | PASS | 95 | 2105 |
| S24 | PASS | 32 | 941 |
| S25 | PASS | 16 | 446 |
| S26 | PASS | 15 | 480 |
| S27 | PASS | 11 | 337 |
| B12 (Slack post) | NOT_RUN | 0 | 0 |
| B12a (Slack thread) | NOT_RUN | 0 | 0 |

**Totals: 27 PASS, 0 PARTIAL, 0 FAIL, 2 NOT_RUN. 811 commands, 21,535 ms
wall-clock** — a mean of 30.0 commands and 798 ms per scenario. The two
NOT_RUN rows require an authenticated Slack workspace, so they are never
counted as passes. Which make target produced each row, and which scenario
file it runs, is listed under “Running the benchmark” below.

### Live, read-only (network allowed)

| Scenario | Verdict | Commands | wall_ms | What it did |
|---|---|---|---|---|
| B22 (S29) | PASS | 6 | 717 | the same constant on three Wikipedia articles |
| B23 (S31) | PASS | 6 | 571 | two articles; the second heading is asserted to differ from the first |

Both are read-only and were run on 2026-11-16. Wikipedia content changes, so
these two rows are not reproducible bit-for-bit; what is reproducible is the
assertion shape (same value on every source; the headings differ). Neither row
maps to a browser-use task ID — their task set is encrypted and no correspondence
is claimed.

### Live, writes (Slack, throwaway workspace)

Run on 2026-11-16 against a freshly created free workspace (`htrcli1`, team
`T0CJH11BT60`), channel `#new-channel`, which Slack reports as visible only to
its creator. These were driven interactively over the extension transport, not
through `bench run`, so they carry no `commands`/`wall_ms` metric — the evidence
is the assertions below, each read back from the page.

| Task | Verdict | What was done | Evidence read back |
|---|---|---|---|
| B12 Post a message | PASS | typed into the channel composer and sent | message text present in the channel; composer empty afterwards |
| B12a Reply in a thread | PASS | opened the message's thread and replied there | reply present in the thread panel; the **channel** body does not contain the reply text; the parent shows **"1 reply"** |

The B12a row is the one that matters: a reply that silently posts to the channel
would fail the `channelHasReplyText: false` assertion, so the check discriminates
between "threaded" and "not threaded".

The test message and its thread reply were **left in place** in that throwaway
workspace as the evidence; the channel is private to its creator and no other
member was invited. There is no repeatable `bench run` scenario for these two:
`scenarios/s28-slack.json` stays `requires_auth` and therefore `NOT_RUN` offline,
so a scripted live version would need a second, non-auth scenario file.

### Live, writes (Google, personal accounts — 2026-11-17)

Account `c00lways@gmail.com` (Chrome, extension transport, tab 1100668751) writing
to `james@mercstudio.com` (Firefox, tab 71). Both accounts belong to the user and
the only recipient was the user's second address, so nothing left their control.
Driven interactively over the extension transport, not through `bench run`, so
these rows carry no `commands`/`wall_ms` metric.

| Task | Verdict | What was done | Evidence read back |
|---|---|---|---|
| Create a Drive folder (B10 sub-capability) | PASS | New → New folder, named it, confirmed | the folder appears in the My Drive listing |
| Send a new message (B02 sub-capability) | PASS, caveat | composed a labelled test message and sent it | "Message sent" toast; recipient inbox total moved 72,004 → 72,005; top row is the test subject |
| B08 archive sub-capability | PASS | opened a promotional newsletter ("November at DeepInfra…") and archived it | `in:inbox from:deepinfra` → 8 hits, the November mail absent; `in:anywhere` → 9, it is present |
| B10 Drive move | NOT_RUN | could not reach "Move to" (see below) | no move performed; two test folders left behind |
| Type a recipient into Gmail compose | **FAIL (harness gap)** | `fill`/`type` on the To field | never succeeded — the field is 0×0 except immediately after a click |

**The send caveat is the interesting part.** Gmail renders the recipient field
inside a zero-size hidden `table.GS` (`input.agP.aFw`, `aria-label="To
recipients"`, 0×0). Clicking the visible placeholder does focus that input, but it
collapses back to 0×0 and unfocused before the next htrcli invocation can type, so
the harness has no way to act on an input that only exists between two commands.
Recipients were supplied through Gmail's `?view=cm&fs=1&to=…` compose view
instead; the send, the Sent copy and the delivery were then all real and read back
from the page. This row is **FAIL, not NOT_RUN** — it was attempted and it failed.

In this Gmail build the archive button's accessible name is `Remove label Inbox
from this conversation`, not `Archive`; a selector written against an older build
will not match.

Drive UI move is NOT_RUN for these reasons, each observed directly:

- the file context menu is created in the DOM but positioned at `y=774` with a
  height of 233 while the viewport is `1151×774`, i.e. entirely below the fold, so
  every item click lands outside the viewport;
- the per-row ⋮ (`aria-label="More actions (Alt + A)"`) has a 0×0 rect until the
  row is hovered — `hover xy=` now supports a coordinate target, but that fix lives
  in `src/contentScript` and needs an extension rebuild before it helps live;
- `rightclick` was absent from the CLI (`rightclickCmd` added 2026-11-17). The move
  path that does work up to the last step is: click the row to select it, click the
  selection bar's `More actions`, then the `Organise` submenu — but until
  2026-11-18 `click "text=Organise"` resolved a page wrapper instead of the menu
  item, so the submenu never opened. `htrcli do` plus the innermost-match fix should
  carry it in one round trip, and it is still **NOT_RUN** until a reloaded extension
  lands the folder — claiming it before that would be a guess;
- the new-folder dialog's Create button moves between openings (y=478, then
  y=452), so a coordinate read once goes stale;
- Drive's `c` then `f` shortcut did not open the dialog;
- `screenshot` returns `504 screenshot timed out` whenever the Chrome tab is not
  the focused one.

Left in place: folders `htrcli-move-test` and `htrcli-move-dest` at My Drive root.
They are the only residue from this run and are safe to delete by hand.

### Judged run (real local model)

| Scenario | Verdict | Score | Threshold | Judge |
|---|---|---|---|---|
| S30 JUDGE-DEMO | PASS | 100 | 60 | Qwen3-0.6B-Q8_0 (`sha256 9465e63a…b031`) on llama.cpp `llama-server` 0.7.0-dev (commit a469940), temperature 0, `JUDGE_AUTH=none` |

The judge error path is itself exercised: an earlier attempt with the model
server down reported `judge error: … failed (curl exit 7, HTTP 000)` and FAILED
the scenario — never a score of 0.

**Scope of this row: plumbing and weighting, not accuracy.** A 0.6B model scoring
a two-item rubric on a trivial scenario shows the adapter, the JSON contract and
the weighted arithmetic work end to end. It says nothing about grading quality;
see “Judge validation” for the one discriminating run (100 vs 50) and the
standing caveat.

### What these numbers mean, and what they do not

`commands` counts htrcli invocations and `wall_ms` is harness-observed time.
This is **harness overhead**, not agent competence: no model chose any step in
the offline rows. Compare the shape with the Browser Use table below, never the
absolute numbers.

A compound `do '[…]'` run counts as **one** command however many steps it carries,
because it is one page round trip and one harness decision. That is deliberate:
the metric is meant to price what the caller had to decide, and "open the menu,
then click Move" is a single intent even when it is two interactions. Steps are
visible inside the invocation's own output, so a `do` step count is never hidden
from a reader who wants it.

## Gap sourcing

Likely-gap notes are inferred from htrcli's actual capabilities as documented
in `skills/htrcli/SKILL.md` and its command set (e.g. no DOM-level `wait`
primitive exists — only `network wait`; contenteditable fields require `type`
not `fill`; virtualized lists need scroll+re-extract; `--frame` is one level
and same-origin). They are hypotheses to be validated by running the
scenarios, not assertions.

## Scenarios (P0 first)

### B01 — Find emails (Gmail) — P0
- **App**: Gmail web
- **Goal**: Search unread mail from a sender/subject; return structured list
  (sender, subject, date, snippet).
- **Start state**: logged-in inbox; query string provided.
- **Pass/fail**: ≥1 result with sender+subject+date; no wrong-folder rows.
- **Likely htrcli gaps**: PARTIALLY resolved. `wait` (DOM condition),
  `collect` (virtualized list) and `extract list --fields` all exist and work
  against the local fixture (scenario S02), but the fixture hides two defects a
  live inbox exposes.
- **Live run (2026-10-27, read-only, extension transport)**: search + structure
  PASS, the "no wrong-folder rows" criterion FAILS.
  `fill "input[name=q]" "is:unread"` then `press Enter` drove Gmail's own search
  (`#search/is%3Aunread`; Gmail's header read "1–37 of 37"), and
  `extract list "tr.zA" --fields "sender:span.yP,subject:span.bog,date:@td.xW span@title"`
  returned structured `{sender, subject, date}` rows. Two measured defects:
  (1) **stale rows** — Gmail's SPA leaves the previous view in the DOM, so
  `tr.zA` matched **87** rows for 37 conversations. The 37 VISIBLE rows were the
  correct unread results, but 50 HIDDEN rows remained from the previous view:
  36 of them read (a stale-state violation of `is:unread`) plus 14 duplicate
  unread rows, giving 73 unique thread ids across 87 rows. A visibility filter
  is required; no CSS selector alone fixes it.
  (2) **one selector per field** — live Gmail varies markup by row state:
  sender is `span.yP` (carrying an `email` attribute) or `span.zF`, subject is
  `span.bog` (read) or `span.bqe` (unread), the date is `td.xW span[title]` or
  `span.bq3`. Only **36 of 50** inbox rows came back with all three fields.
  Field-form note: a descendant attribute is written `@<css>@<attr>`
  (e.g. `@td.xW span@title`). The help's `@.link@href` example reads as a `@.`
  prefix but the dot is the CSS class marker
  (`src/contentScript/extractData.ts:283`); a valid-but-unmatched selector
  yields `""` rather than an error.
  **Both defects are now fixed in code** (`extract list --visible`;
  `sender:span.yP||span.zF` alternatives), verified end-to-end over CDP against
  a probe page (3 rows with one inside `display:none`: 3 returned without the
  flag, 2 with it). The collector had the same hole and
  now takes `--visible` as well (`collect` runs many harvests, so it could
  accumulate stale rows mid-scroll as well as at the start).
- **Live re-check (2026-10-28, after an extension reload)**: PASS. The control
  run without the flag returned **87** rows; `--visible` returned **37**, equal
  to Gmail's own "1–37 of 37", and **37 of 37** rows had non-empty sender,
  subject and date (previously 36 of 50). `collect "tr.zA" --visible` returned
  the same **37** in `0` scroll steps with `truncated: false`. Read-only
  throughout: no send/delete/archive/mark/draft, and the view was restored to
  `#inbox`.
- **Metrics**: success, commands, wall_ms, retries, failure_mode.

### B02 — Reply to an email (Gmail) — P0
- **App**: Gmail web
- **Goal**: Open a thread, reply with a provided body, send, confirm delivery.
- **Start state**: thread located by subject.
- **Pass/fail**: reply in Sent; thread shows new message; "Message sent" toast.
- **Likely gaps**: RESOLVED — `type` now appends to a contenteditable (it
  previously wiped the quoted text and signature); `fill` still replaces, so
  it must not be used on a reply box.
- **Safety**: reply to yourself or save as draft.

### B03 — Arrange a meeting (Google Calendar) — P0
- **App**: Google Calendar web
- **Goal**: Create event (title, attendees, date/time, link), send invites.
- **Start state**: logged-in calendar.
- **Pass/fail**: event visible with correct title/time/attendees; no error toast.
- **Likely gaps**: date/time pickers need `type` + read-back; guest
  autocomplete needs `pick` with full option text. Both verified against the
  local Calendar fixture (scenario S04).

### B04 — Create todo / reminder — P0
- **App**: Google Tasks or Todoist web
- **Goal**: Add task with title, due date, priority, optional reminder.
- **Start state**: app open.
- **Pass/fail**: task appears with correct due date.
- **Likely gaps**: RESOLVED — two gaps, both found and fixed against the
  local Tasks fixture (scenarios S07, S08):
  1. **Quick-add vs field entry.** Both paths now work: quick-add is a single
     field committed with `Enter`; the full form is a details panel whose
     date is a **popover grid of day buttons** (no date field exists).
  2. **The completion checkbox is an ARIA checkbox**
     (`<div role="checkbox" aria-checked>`), not an `<input type=checkbox>`.
     `check`/`uncheck` previously failed with "Element is not a
     checkbox/radio input"; they now drive `aria-checked` and dispatch a
     click, and are idempotent.
- **Also fixed**: `extract list` could only read a field's TEXT, so a task's
  due date (an attribute) came back empty. Field selectors now support
  `@attr` (the item's own attribute), `@sel@attr` (a descendant's attribute)
  and `[data-x]` (an attribute value).

### B05 — Google Docs: create + edit + share — P1
- **App**: Google Docs web
- **Goal**: Create doc, add headings/paragraphs/bullets, share (view/comment/edit).
- **Start state**: logged-in Drive/Docs.
- **Pass/fail**: doc contains requested content; share dialog confirms.
- **Likely gaps**: Docs is canvas-ish; typing via `type`/`press` only;
  `fill` won't work on editing surface. Verified against the local Docs
  fixture (scenario S05).

### B06 — Google Sheets / Excel Online: data entry + formula — P1
- **App**: Google Sheets or Excel Online
- **Goal**: Enter rows of data, add SUM/lookup formula, verify computed value.
- **Start state**: blank sheet.
- **Pass/fail**: all cells match input; formula result matches expected.
- **Likely gaps**: cell navigation (Tab/Enter) scripted; virtualized grid.
  Cell entry and grid read-back verified against the local Sheets fixture
  (scenario S06).

### B13 — Form filling (HR/IT/surveys) — P2
- **App**: web form
- **Goal**: Fill multi-page form end-to-end, submit, capture ref.
- **Start state**: form page 1 open.
- **Pass/fail**: all fields match; submission ref captured; no validation error after correct fill.
- **Likely gaps**: multi-page navigation, required-field validation, selects/comboboxes. OFFLINE PROXY verified via S09 (docs/benchmarks/fixtures/form.html): name+email fill, date fill, category select, country pick, confirmation reference (EXP-100001), and error cleared after correct fill. Live form run needs an actual expense portal.
- **Safety**: test data only; never enter real personal data.

### B07 — Booking (meeting room / travel / appointment) — P1
- **App**: booking site
- **Goal**: Book a resource for a time slot; capture confirmation number.
- **Start state**: search form available.
- **Pass/fail**: confirmation number captured; booking appears in account.
- **Likely gaps**: date pickers, seat maps; payment gating (never enter card). OFFLINE PROXY verified via S10 (docs/benchmarks/fixtures/booking.html): room select via native `<select>`, date via popover day grid, time select, seat map with one taken seat, confirmation number captured. Live booking-site run needs auth — see BLOCKED line in TODO.md.
- **Safety**: use a clearly-labelled test slot; cancel after capture.

### B08 — Forward / label / archive email — P1
- **App**: Gmail web
- **Goal**: Forward a thread with a note; apply label; archive.
- **Pass/fail**: forwarded in Sent; label applied; thread archived.
- **Likely gaps**: forward compose, custom label popup (not a native
  `<select>`), and proving a row LEFT the page after archive (nothing in
  `wait` could assert absence before `--gone`). OFFLINE PROXY verified via S13
  (`fixtures/mail.html`): forward to `dave@example.com` with a typed note (sent
  log records recipient + exact note), a `check` of the Finance checkbox in a
  custom label menu (row 2 `data-labels=Finance`), and an archive asserted with
  `wait --gone` (row 3 absent from the DOM, counter 1, 2 rows left, row 1
  untouched). Live Gmail run still needs auth.

### B09 — Expense report / invoice submission — P2
- **App**: expense portal
- **Goal**: Fill expense form with line items (date, category, amount, receipt
  upload), submit.
- **Pass/fail**: submission confirmation; totals match.
- **Likely gaps**: receipt upload, multi-line items. OFFLINE PROXY verified via
  S11 (`fixtures/expense.html` + `fixtures/receipt.txt`): dynamic add-line,
  native `<select>` per row, `htrcli upload` on a real file input (filename and
  size read back), running total (125.50 + 42.75 = 168.25), and confirmation
  reference `EXR-700001`. Live expense-portal run still needs auth.

### B10 — File management (Drive/OneDrive/Dropbox) — P2
- **App**: cloud drive web
- **Goal**: Upload a file, move to folder, share link, set permissions.
- **Pass/fail**: file in target folder; share link works.
- **Likely gaps**: file move (often native drag-and-drop, which is not
  synthesized), share dialogs, permission selects. OFFLINE PROXY verified via
  S12 (`fixtures/files.html` + `fixtures/report.txt`): `htrcli upload` on a real
  file input (filename + real byte size 72 read back), a click-based move via a
  per-row native `<select>` (avoiding unsupported HTML5 drag-and-drop), a DOM
  share dialog, the share link read back with `value`
  (`https://files.example/s/ABC123`), edit permission granted, and an untouched
  file confirmed unchanged. Live Drive/OneDrive run still needs auth.

### B11 — Contact / CRM update — P2
- **App**: CRM web
- **Goal**: Find contact, update fields (phone, note, tag), log activity.
- **Pass/fail**: fields persisted; activity logged.
- **Likely gaps**: live search filters often HIDE rows rather than remove them
  (then `wait --gone` times out — assert the surviving count instead); tag
  inputs commit on Enter into chips and have no native control; `fill` cannot
  set an empty string, so clearing a search box needs `clear`. OFFLINE PROXY
  verified via S14 (`fixtures/crm.html`): filter to one contact (non-matching
  rows removed from the DOM, proven with `wait --gone`), clear with `clear`,
  update phone + note, commit two tag chips with `fill` + `press Enter`, save,
  reopen to prove the values came from the data model (not the form), log an
  activity attributed to the edited contact, and confirm a third contact was
  untouched. Live CRM run still needs auth.

### B12 — Messaging (Slack/Teams/Discord web) — P2
- **App**: messaging web
- **Goal**: Post a message to a channel/DM, react, search history.
- **Pass/fail**: message visible; search returns it.
- **Likely gaps**: the composer is `contenteditable` (Slack/Teams), so `fill`
  replaces the text and `Enter` must send while `Shift+Enter` newlines — a
  composer that swallows Enter needs a click on the send button instead; a
  thread adds a SECOND contenteditable to the page, so the reply must be typed
  into the thread input and must land on its parent rather than the channel —
  split out as B12a; reactions are buttons whose state lives in the app's model,
  so the real question is whether it survives a re-render. OFFLINE PROXY
  verified via S15
  (`fixtures/messaging.html`): react ✅ on a message that already shows two 👍
  from other people (count above zero is not the same as reacted-by-me), send
  from the contenteditable with `press Enter`, prove the ✅ survives the
  re-render that sending causes, search "budget" (matching message stays, the
  others leave the DOM, proven with `wait --gone`), clear with `clear`, post to
  a DM with the Send button, then switch back and prove neither write leaked
  into the other conversation. Live Slack/Teams run still needs auth.

### B12a — Thread replies (messaging) — P2
- **App**: messaging web
- **Goal**: Reply to a specific message inside its thread, not the channel.
- **Pass/fail**: the reply attaches to the opened parent; channel history is
  unchanged.
- **Likely gaps**: a thread is a different WRITE TARGET that looks identical to
  the channel composer — both are `contenteditable`, both send on Enter, and the
  only thing distinguishing them is which input has focus. The failure this
  catches is a reply that renders in the thread optimistically but is committed
  to the channel (or to the wrong parent). OFFLINE PROXY verified via S17
  (`fixtures/messaging.html`): two parents start with different reply counts
  (m1=0, m2=2) so a mis-attached reply shows as a numeric mismatch, a reply typed
  into `#thread-input` and sent with Enter raises m2 to 3 while the channel list
  stays at three messages and never contains the reply text
  (`stdoutNotContains`), the count survives a switch to another channel and back
  (model-backed, not DOM-only), and a second reply to m1 proves counts are
  per-parent (m1=1, m2 unchanged). `extract list` errors on zero matches by
  design, so an empty thread is asserted with `wait --selector
  "#thread-replies:empty"`. Live Slack/Teams run still needs auth.
- **Safety**: reply in a test channel only.

### B14 — Research & summarize (web) — P2
- **App**: web pages
- **Goal**: Open N pages, extract key facts, produce a summary.
- **Pass/fail**: summary cites sources; no hallucinated facts.
- **Likely gaps**: this is the first task whose steps must CARRY data — a fact
  read on page A has to appear in a summary written on page C — so the harness
  grew step-output capture (`"capture": {"name": "items.0.q4"}` on the extracting
  step, `${name}` in a later argv; see "Running the benchmark"). "No hallucinated
  facts" cannot be proven offline by a browser at all: the offline proxy proves
  the WORKFLOW (read → carry → cite → save), never the truthfulness of prose.
  OFFLINE PROXY verified via S16 (`fixtures/research-source-a.html`,
  `research-source-b.html`, `research-summary.html`): read a Q4 figure plus an
  **opaque** reference (RPT-8841 from A, RET-2260 from B) from two separate
  pages, compose a summary substituting all four captured values, assert no
  literal `${` survived, get rejected first for an empty summary and then for a
  written summary with no source, cite both pages, save, and read back the exact
  sentence plus a `stdoutNotContains` guard proving Q3's figure was not mistaken
  for Q4's. A live research run still needs target sites that permit reads.
- **Live run (2026-10-28, read-only, public pages, `--transport cdp`)**: PASS on
  the mechanics, and on the half of "no hallucinated facts" a browser can
  actually check. Two real encyclopedia pages with exact, quotable values —
  `en.wikipedia.org/wiki/Speed_of_light` and `.../Avogadro_constant` — were read
  in a dedicated Chrome profile: no account, no writes, nothing typed into a
  page. Per page, `wait --selector "table.infobox"` then
  `extract table "table.infobox"` returned a structured
  `{columns, rows, rowCount}` payload (23 rows on the first). Captured and
  cited: A → `299792458` m/s, B → `6.02214076 × 10^23 mol^-1`.
  Reproduce (no fixture, no account; `--cdp` needs `htrcli browser start`):
  ```bash
  htrcli --cdp open "https://en.wikipedia.org/wiki/Speed_of_light"
  htrcli --cdp wait --selector "table.infobox" --timeout 20000
  htrcli --cdp extract table "table.infobox" --json        # 23 rows; exact value in col2
  htrcli --cdp eval "document.querySelector('#mw-content-text').innerText.includes('299792458')"  # true
  htrcli --cdp eval "document.querySelector('#mw-content-text').innerText.includes('299792459')"  # false
  # then the same three over https://en.wikipedia.org/wiki/Avogadro_constant
  # with 6.02214076 (true) and 6.02214077 (false).
  ```
  **Attribution audit** (what replaces "prove the prose is true", which no
  browser can do): for each cited value the source page's own text was re-read
  in page context and the claim re-tested —
  `eval "document.querySelector('#mw-content-text').innerText.includes('<digits>')"`.
  It returned `true` for `299792458` on A and `6.02214076` on B, and `false` on
  both pages for the perturbed values `299792459` / `6.02214077`, so the audit
  discriminates instead of agreeing with whatever it is given. **Prose
  faithfulness: NOT PROVEN** — no browser can check that a summary reasons
  correctly over correct citations, so the pass condition for a live run is
  "every number and reference traces to a live page", never "the text is true".
  **Two real-page fidelity traps the fixture never produced**:
  1. the extracted cell for A is `"299792458\u200d[2]"` — a zero-width joiner
     plus a footnote marker, so the digits must be stripped before citing;
  2. B's exponent is **flattened** by `textContent`: `6.02214076×10<sup>23</sup>`
     comes back as `"6.02214076×1023"`, which reads as 6.02×10^1023 if quoted
     verbatim. The mantissa survives; the exponent must be recovered from the
     DOM.
  Both are content traps rather than extraction failures — the DOM text really
  is that — but a research step must normalize before trusting a number. See
  the TODO entry about number fidelity in `extract table`.

### B15 — Social media post / schedule — P3
- **App**: social platform web
- **Goal**: Compose and (optionally) schedule a post with media.
- **Pass/fail**: post visible on profile / scheduled in queue.
- **Likely gaps**: rich-text/media upload, scheduling date pickers.
- **Likely gaps (as measured)**: the character limit is the interesting part,
  not the composer. A scenario that only checks the counter proves nothing —
  the counter is a different code path from the submit validator, so a `>=`
  off-by-one inside the validator passes a counter-only scenario (this was a
  real surviving mutant until the scenario submitted a post of EXACTLY the
  limit and required it to be accepted). The other trap is time: "schedule for
  a future time" depends on now, so the fixture carries a FIXED demo clock
  (`data-now="2026-04-01T09:00"`) and compares against it, which makes the
  past / exactly-now / future boundaries deterministic. Native
  `<input type="datetime-local">` accepts `fill` and normalizes to
  `YYYY-MM-DDTHH:mm`; its `min` attribute is documentation only — it does not
  block a programmatic fill, so the page's own check is what the scenario
  asserts. Media upload reuses the `upload` path already proven by S12/S18
  rather than adding a binary fixture. OFFLINE PROXY verified via S20
  (`fixtures/social.html`): empty-composer refusal from both buttons, 300-char
  and 281-char refusals quoting exact lengths, 280 characters accepted and
  stored WITHOUT silent truncation, missing-time / past-time / exactly-now
  refusals, a scheduled post (audience + attachment) vs an immediately
  published one, both surviving a reload with distinct states, and a
  reschedule IN PLACE that must not duplicate the post — with the seeded post
  untouched throughout. Live social platform run still needs auth.

### B16 — E-commerce reorder — P3
- **App**: store account web
- **Goal**: Reorder a previous purchase; capture order number.
- **Pass/fail**: order confirmation number captured.
- **Safety**: use test store / cancel if possible; never enter real card.
- **Likely gaps**: a re-order is not a copy of an order, and the benchmark has to
  be able to tell the difference. Three observable state facts decide it: a NEW
  order id (never the source id), exactly one additional order in the history
  with the source row unchanged, and a cleared cart — "items re-added to the
  cart but never checked out" must not read as done. Prices and quantities part
  company: the quantities carry over from the old order, the prices come from
  TODAY's catalogue. A fixture whose price never moved cannot distinguish those,
  so the history keeps what was PAID per line while the catalogue carries the
  current price, and the two disagree on one line. A discontinued line is the
  third case: still visible in the cart (with the reason on the page), excluded
  from the subtotal, and absent from the review page.
  Safety is part of the task, not a footnote: a decoy "Buy again (1-click)"
  button sits beside "Reorder" and would place an order with no cart, address or
  review step. The scenario clicks Reorder, and proves the decoy was reachable
  but never used (an audit log that records it, plus write counters) and that
  the new-card form was never opened (saved test token only, last four digits
  shown). A two-step cancel is then exercised, including backing out of it.
  OFFLINE PROXY verified via S23 (`fixtures/store.html`): a fresh run clears the
  seeded state, then re-orders ORD-1042 — two buyable lines with quantities 2
  and 1, the discontinued organiser shown as unavailable and absent from the
  subtotal, the pen pack priced $7.25 today against $6.00 in the history,
  subtotal $24.25 + $4.95 shipping = $29.20, a quantity edit to 2 recomputing to
  $31.50/$36.45 and back, checkout gated one field at a time (address, then
  payment, each naming itself), the review page dropping the unavailable line
  and showing the grand total plus the office address and `•••• 4242`, order
  ORD-2031 captured from the confirmation, one write and zero 1-click entries,
  the source order still $42.95/Delivered, then a cancel that must be confirmed
  (Keep order leaves it Processing; confirming makes it Cancelled, removes the
  control, keeps the row) and survives a `reload`. htrcli needed no new command:
  `check` drives the radios, `--json value` reads the carried quantities,
  `--json attr` reads the status/write counters, and `extract table|list` covers
  the cart/review/history counts. 13 mutants caught, including reorder-mutates-
  the-source-order and reorder-reuses-the-source-id. Live store run stays
  BLOCKED (test store or cancel; never a real card).

### B17 — Support ticket create / update — P3
- **App**: helpdesk web
- **Goal**: Create ticket with subject/body/attachment; add a comment.
- **Pass/fail**: ticket number captured; comment visible.
- **Likely gaps**: this is the first task with a real create→update CHAIN — the
  generated id has to be captured and then used as a SELECTOR to reopen the
  record, so the harness must expand captures inside expectations too (see
  "Running the benchmark"), not only inside argv. A ticket form is otherwise
  ordinary (text, textarea, two selects, a file input), but the update half is
  not: status and assignee are separate writes that must be re-read from the
  model after a view change, and a rejected update must not persist. OFFLINE
  PROXY verified via S18 (`fixtures/ticket.html` + `report.txt`): two seeded
  tickets with different statuses/assignees, create rejected twice for missing
  subject then missing description, subject/description/category/priority/
  attachment submitted, the generated id captured and then used as
  `[data-ticket-id='${ticket_id}']`, resolving without an assignee refused,
  status+assignee updated and read back from the list, a comment posted and its
  count reflected on the row, and both seeded tickets proven untouched. Live
  helpdesk run still needs auth.

### B18 — Bank / finance portal read-only check — P3
- **App**: bank web
- **Goal**: Read balance and recent transactions; no writes.
- **Pass/fail**: balance + ≥1 transaction captured.
- **Safety**: read-only; never initiate transfers.
- **Likely gaps**: the hard part is asserting a NEGATIVE. "No write happened" is
  not provable from the absence of a confirmation — an unreachable or misspelled
  selector is absent too, and passes. The page therefore has to record its own
  READS (a live access log is the positive control) while the write path stays
  present and one click away but unused, and the log must distinguish the two so
  a `transfer` entry would be visible if one occurred. Two more traps: the
  CURRENT and AVAILABLE balances are different fields (reading the wrong card
  looks right to the number of decimals), and a period filter must change the
  rows and the period total but never the balance. The running-balance column is
  the reconciliation check: its last value must equal the header balance, so a
  table that is merely present cannot pass.
  OFFLINE PROXY verified via S22 (`fixtures/finance.html`): a fresh run clears
  the fixture ledger first, then reads $4,820.50 (and 4820.50 via
  `attr data-amount`, captured and reused in later expectations) with a
  deliberately different available balance, six rows in the default period with
  the 28 March row provably absent, the March row reappearing with a wider
  filter and the period total moving +$520.50 → -$117.50 → +$460.50 while the
  balance never moves, `extract table` reading headers + rows, the last running
  balance reconciling with the header, a second account ($12,483.07) proving the
  reads are account-scoped and its predecessor's rows gone, and then the
  read-only half: the access log records every view/filter (so the log is
  demonstrably live and the `tr[data-action='transfer']` selector is able to
  match), `body[data-writes]` stays 0, the transfer panel is opened but never
  submitted, and after a `reload` the balance, the six-row default period, the
  log and the zero write count are all unchanged. htrcli needed one real fix
  here: `os.Expand` read the `$4` in `"$4,760.50"` as a variable named `4` and
  rewrote expectations to `"${4},760.50"` as soon as a capture existed — see
  `expandBenchRef`. Live bank run still needs auth (read-only, and only against
  a staging account).

### B19 — HR leave request — P3
- **App**: HR portal
- **Goal**: Submit a leave request with dates and reason.
- **Pass/fail**: request number/status captured.
- **Likely gaps**: the first scenario whose correct answer is a NUMBER the page
  computes, so a wrong implementation still produces a plausible-looking form.
  Three things have to be pinned separately: the day count is INCLUSIVE of both
  endpoints and of weekends (off-by-one and "business days only" are both
  wrong-but-reasonable), the balance is read-modify-written in both directions
  (a request takes days, a cancellation gives them back), and the result
  survives a reload — which needs `reload` (it works on both transports; every
  bench scenario runs over `--cdp`). That last one was a hard stop: `reload`
  used to answer "reload is not supported over --cdp yet", so no scenario could
  prove persistence at all. It is now implemented (Page.reload + wait for the
  load event) and this scenario is its first user. The date fields are native
  `<input type="date">`, matching what real portals use; `fill` sets them and
  the browser normalizes the value, and a malformed string silently becomes
  empty, so the scenario also asserts the normalized value read back.
  OFFLINE PROXY verified via S19 (`fixtures/leave.html`): a fresh run clears the
  fixture's storage first (the scenario proves persistence, so it must not
  inherit the previous run), then walks a half-filled range, the live duration
  for a weekend-spanning Monday→Sunday week (7 days, not 6 and not 5), a
  single-day range, an inverted range shown as no duration and refused on
  submit, missing type, missing reason, a 30-day request refused against a
  14-day balance with no row and no confirmation, a fitting 7-day request
  (balance 14 → 7, captured request id), the reload, and a cancellation that
  restores 14 and still reads cancelled after a second reload — with the
  pre-existing approved request untouched throughout. Live HR portal run still
  needs auth.

### B20 — Survey / feedback submission — P3
- **App**: survey web
- **Goal**: Complete a multi-page survey and submit.
- **Pass/fail**: completion confirmation captured.
- **Safety**: answers only; never submit a survey on someone else's behalf.
- **Likely gaps**: the first task where the ARTIFACT is a set of answers rather
  than one record, so a submission that looks complete can still carry the wrong
  ones. Three things have to be pinned separately: the wizard gates per page AND
  per question (each unanswered question is named, and answering a later one
  must not satisfy an earlier one), a rating matrix must record each ROW
  independently (one listener keyed to a single row renders identically but
  writes every row into the same field), and going Back must restore what was
  already answered — which only holds if the step re-renders FROM THE MODEL. A
  step that is merely hidden keeps the DOM's own values, so the restore is
  untestable; here that mutant SURVIVED until the fixture began clearing its
  inputs before every render. The free-text minimum belongs at the exact
  boundary (9 refused, 10 accepted), and a submitted response must be refused a
  second time and still carry the same id after a reload.
  OFFLINE PROXY verified via S21 (`fixtures/survey.html`): a fresh run clears
  the fixture's storage first (the scenario proves a submission survives a
  reload, so it must not inherit the last run), then Next-with-nothing names
  only the first missing question, answering work mode advances the message to
  Team size, a Back/forward round trip proves the radio and the select were
  restored from the model (`:checked` + `--json value`), the matrix names
  Onboarding while Tooling is answered and stays on Onboarding after `check`
  Tooling, the multi-select gate appears once the matrix is complete, the review
  page reflects every answer (and BOTH tools, not the last one clicked), a
  checkbox removal and a matrix edit made from the review are reflected without
  disturbing neighbouring answers, 9 characters is refused at the boundary and
  10 accepted, the capture id appears in the response list, a second submit is
  refused with the row count unchanged, and after `reload` the same id is shown
  with `body[data-submitted='1']` and the seeded response still reads Hybrid.
  htrcli needed nothing new: `check` drives `<input type="radio">` through the
  native `checked` setter, `wait` matches `:checked` and attribute selectors,
  `select` takes the option VALUE, and `--json value` reads it back. One
  documented GAP was worked around: "the error area is empty" is not assertable
  (`--json` drops an empty `data`), so the successful submit is proven by the
  confirmation and the row count instead. Live survey run still needs auth.

## Enhancement backlog (driven by gaps above)

- [x] DOM-level `wait` command (selector/text/URL) for both transports (htrcli wait).
- [x] Rich-text (contenteditable) typing helpers.
- [x] Virtualized-list extraction helper (`htrcli collect` — scrolls the container, accumulates + de-dupes rows).
- [x] Retry-with-backoff wrapper for flaky selectors (`htrcli retry -- <cmd>`).
- [x] Structured data extraction (`extract table/list`) command.
- [x] Skill doc updates for each office app (Gmail, Calendar, Docs, Sheets) — see
  `skills/htrcli/SKILL.md` → "Office apps: Gmail, Calendar, Docs, Sheets".
- [x] Benchmark harness: run scenario, capture pass/fail/time, write report (`htrcli bench run`).
- [x] Attribute fields for `extract list` (`@attr`, `@sel@attr`, `[data-x]`) — a task's due date is an attribute, not text.
- [x] ARIA checkbox support for `check`/`uncheck` (`role="checkbox"` + `aria-checked`).
- [x] Step-output capture + `${var}` substitution (`"capture": {"name": "items.0.q4"}`), plus `${VAR}` expansion in expectation strings — the prerequisite for any scenario that aggregates across steps (B14).
- [x] `reload` over `--cdp` (`cdp.Reload`: Page.reload, then wait for
  `Page.loadEventFired`). Previously "not supported over --cdp yet", which
  blocked every persistence assertion — a benchmark can only prove state was
  persisted by destroying the page's memory first. Deliberately Page.reload and
  not a re-navigate of the same URL, which can be served as a same-document
  navigation and leave the old JS state alive. Pinned by
  `internal/cdp/nav_reload_test.go` (verb + wait).
- [x] Captures expand inside expectations (`stdoutContains`, `stdoutNotContains`,
  `jsonPath.equals`), so a scenario can assert that a record opened on the id it
  just captured (B17).
- [x] Visibility filtering for both list verbs (`extract list --visible`,
  `collect --visible`). An SPA keeps its previous view in the DOM — live Gmail
  answered 87 `tr.zA` matches for 37 conversations — so a list read has to drop
  unrendered rows instead of reporting them as data. `collect` re-checks at
  every harvest, so a row hidden on one pass and rendered on a later one is
  still collected once.

## External alignment: browser-use BU Bench V2.1

### Source and licence constraint

The `browser-use/benchmark` repository publishes BU Bench V2.1: 200 tasks
scored against weighted findings rubrics, with a continuous score and partial
credit, judged by an LLM that receives the full rubric, the saved trajectory and
the per-task screenshots. Its task dataset is encrypted (Fernet,
`BU_Bench_V2.enc`), and `Stealth_Bench_V1.enc` holds the CAPTCHA and anti-bot
tasks. The README states: "The tasks are encrypted to keep their text out of web
crawlers and model training data. Please do not publish decrypted tasks or
rubrics in plaintext or use them for model training."

We adopt the structure only — taxonomy, protocol and scoring model. No task text
is imported, and their dataset files must not be added to this repository.

### Adopt / reject

| Adopted | Rejected, with reason |
|---|---|
| • Task taxonomy for the four gaps below (B21–B24).<br>• Protocol rules: a hard per-task deadline, one attempt, no silent retry, an exclusions list, per-task evidence capture.<br>• An opt-in weighted-rubric score with partial credit, alongside the existing pass/fail result. | • Their encrypted task text — it cannot be decrypted by us and must not be published or used for training.<br>• A mandatory LLM-judge API key — the harness must stay runnable offline; a judge is opt-in and pluggable.<br>• Their SaaS browser backends — we drive local Chrome/Firefox through CDP or the extension.<br>• A 3,600 s default deadline — our tasks take minutes, not an hour. |

### B21 — Shopping cart → order summary, stop before payment — P1
- **App**: store web (offline fixture cart)
- **Goal**: Add two distinct products, change one quantity, apply a coupon code,
  and reach the order summary without entering a payment step.
- **Proves**: The cart task is the single largest category in their suite (361 of
  500 retained consumer tasks, all of which stop before checkout), and the
  arithmetic is where a browser agent fails silently: a summary page that
  renders can still show a subtotal that does not match its lines.
- **Pass/fail**: Two distinct products added; one quantity changed; the coupon
  applied; the order summary reached; NO payment step attempted; the printed
  subtotal equals the sum of the line items and the discount is arithmetically
  correct.
- **Offline proof (built 2026-11-15)**: S24 (`scenarios/s24-cart.json`, 32
  commands) drives `fixtures/cart.html` through `make bench-cart`. It adds two
  products, changes one quantity, applies SAVE10, and stops at the summary: the
  discount is exactly 10% of $161.00 = $16.10 (1610 cents, not a rounded 16
  dollars), total $144.90. The payment block is proven untouched with a positive
  control — the card field and the reveal button both exist and were not used —
  and `data-payment-revealed` stays 0.

### B22 — Public-information retrieval citing N=3 sources — P1
- **App**: static reference pages
- **Goal**: Answer one factual question from three independent pages, returning
  each URL with the quoted supporting string.
- **Proves**: Public-information retrieval is the other large category in their
  suite (139 of 500), and its failure mode is not a wrong click but a confident
  answer with no traceable source, or a silent pick when sources disagree.
- **Pass/fail**: One answer; three URLs, each with the quoted supporting string;
  when two pages disagree, the run says which it trusts and why.
- **Offline proof (built 2026-11-15)**: S25 (`scenarios/s25-retrieval.json`, 16
  commands) drives `fixtures/retrieval-1.html`..`-3.html` through
  `make bench-retrieval`. All three pages answer the same question; pages 1 and
  3 say 6,000 m and page 2 says 8,000 m. Each page's value and opaque source id
  are captured, so the contradiction stays visible instead of being averaged or
  silently replaced by the last read. The offline proxy proves the disagreement
  is reported; it does not yet assert a trust narrative, which stays a driver
  step.

### B23 — Cross-site comparison table — P2
- **App**: two or three product/offer sites
- **Goal**: Compare the same product or offer across two to three sites and
  produce a ranked table.
- **Proves**: A comparison is the first task whose artifact is a join across
  pages, so a row can be plausible and still untraceable — the failure is an
  invented or carried-over cell, not a missed one.
- **Pass/fail**: A ranked table with price and the deciding attribute for each
  row; no row that is not traceable to a page; an explicit "not stated" where a
  field is absent.
- **Offline proof (built 2026-11-15)**: S26 (`scenarios/s26-compare.json`, 15
  commands) drives `fixtures/compare-a.html` (table markup) and
  `compare-b.html` (list markup) through `make bench-compare`. Both pages use
  the same field names, so a row can be joined across pages; B deliberately does
  not state the weight, and the field is reported empty rather than invented
  (the page's own prose confirms the absence).

### B24 — Anti-bot / CAPTCHA detection and report (never solve) — P2
- **App**: any page serving a challenge
- **Goal**: Recognise a challenge or access-block page, do NOT attempt to defeat
  it, and report the run as blocked with evidence.
- **Proves**: `Stealth_Bench_V1.enc` covers anti-bot behaviour; detection and
  honest reporting are testable without ever solving a challenge, and the
  existing safety rule already forbids solving one.
- **Pass/fail**: The challenge or block recognised; no attempt to defeat it; the
  run reported as blocked with the URL, the page title and the challenge marker.
- **Offline proof (built 2026-11-15)**: S27 (`scenarios/s27-challenge.json`, 11
  commands) drives `fixtures/challenge.html` through `make bench-challenge`.
  The challenge marker and disabled checkbox are detected and reported, no
  content element exists, and an attempted read of the answer fails (exit 1) —
  the refusal is asserted, not just described.

### Harness backlog (built 2026-11-15)

- [x] Built: `deadline_seconds` per scenario — a hard stop reported as TIMEOUT,
  with a per-step override.
- [x] Built: `attempts` policy (default 1). A step's `retry` may not exceed
  `attempts-1`.
- [x] Built: `--evidence-dir` writes one full-output JSON artifact per step. It
  does NOT capture screenshots or page HTML: that would need an extra browser
  call per step.
- [x] Built: `--exclusions <file>` takes `{"excluded":[{"id","reason"}]}`;
  excluded IDs are reported EXCLUDED and counted separately.
- [x] Built: `bench run --judge-cmd` weighted-rubric scoring with partial credit,
  the judge invoked as a local argv so the offline contract holds. Pass/fail
  remains the default.

The offline contract is the default: no network, no API key, and no date or
random dependence.

### Harness schema and flags (built 2026-11-15)

`htrcli bench run` now accepts these scenario fields:

- `requires_network` (bool) — without `--allow-network` the verdict is
  `NOT_RUN`, never PASS. It mirrors `requires_auth` exactly, so a suite cannot
  silently depend on connectivity.
- `usage` `{model,tokens_in,tokens_out}` — DRIVER-supplied token accounting.
  htrcli spends no tokens; it echoes what the calling loop declares. An absent
  `usage` prints an em-dash, never `0`, and a run-level total appears only when
  at least one scenario declares it ("not measured" is not "free").
- `deadline_seconds` — the scenario wall-clock budget; a step may override it.
  A required step that exceeds its deadline is killed and reported `TIMEOUT`.
- `attempts` (default 1) — caps a step's total attempts; a step's `retry` may
  not exceed `attempts-1`.
- `context` — injected as `--context <name>` into every step and included in
  the reported argv.
- `rubric` `[{id,weight,criterion}]` — ids are unique and every weight is > 0;
  declaring a rubric without `--judge-cmd` is a load-time error.

Flags: `--allow-network`, `--judge-cmd` (repeat once per argv element; never a
shell string), `--judge-pass-score` (default 60), `--evidence-dir`,
`--exclusions <file>`, plus the existing `--out`.

Verdicts: `PASS`, `PARTIAL`, `FAIL`, `NOT_RUN`, `TIMEOUT`, `EXCLUDED`. The
summary counts each separately; `NOT_RUN`, `TIMEOUT` and `EXCLUDED` can never
count as PASS.

Judge contract: stdin is
`{"scenario":{"id","goal","app"},"rubric":[...],"evidence":{"steps":[...],"summary":{...}}}`;
stdout is `{"scores":[{"id","score","reason"}]}` with each score in 0..1. The
RUNNER computes the weighted total (0..100, `sum(weight*score)/sum(weight)`). A
non-zero exit, unparsable output, an unknown/missing/duplicate id, or an
out-of-range score is a judge error that FAILS the scenario — never a score of
0. The report keeps the judge argv and the raw stdout. A judged scenario passes
when the weighted score reaches the pass score; only a scenario whose steps
finished PASS or PARTIAL is graded.

Evidence: `--evidence-dir` writes
`<dir>/<scenario-id>/<NNN>-step.json` with full, untruncated stdout/stderr.
Screenshots are NOT captured.

Exclusions: `{"excluded":[{"id","reason"}]}`; an unknown id or an empty reason
is a hard error.

### Fixes in this batch (2026-11-15)

- `--judge-timeout` (default 120 s) bounds the judge process. A judge that
  overruns is a judge error: the scenario FAILS. It is never recorded as a score
  of 0.
- `wait --selector "@eN" --cdp` resolves the ref through the persistent ref
  store (its `backendNodeId`) instead of the page bundle's in-page registry, so
  both the present and `--gone` cases work and the detached-ref path is
  reachable. An unknown ref fails immediately and names it
  (`wait: stale ref: … is not known in the CDP ref store`).

### Fixed defect — retry now forwards persistent flags (2026-11-16)

`retry` used to drop persistent flags set before it (`--transport`, `--json`,
`--tab`, `--token`, `--server`). The cause: `retry.go` read
`rootCmd.PersistentFlags()`, whose pflag `Visit` early-returns because cobra
populates the EXECUTED command's merged flag set, not the root's, so the root
set showed no changed flags and nothing was forwarded. It now reads the
executing command's merged flag set and filters that to persistent flags, so
command-local flags (`--times`, `--delay`, `--max-delay`) are never forwarded
(`htrcli/internal/commands/retry.go`, `inheritedGlobalArgs`).

The vacuous `TestInheritedGlobalArgsForwardsOnlyChangedFlags` (it parsed a
`pflag.FlagSet` and inspected that same set, so it never exercised cobra's
merge) was replaced by a test that drives a real cobra tree; it failed against
the old code. Real-binary proof: `htrcli --json retry --times 0 -- tabs list`
emits JSON, and `htrcli --transport cdp retry --times 0 -- tabs list` returns
CDP target ids (32-hex), not extension tab ids. The eight-fix regression is now
**8/8**.

### Ref document identity — fixed (2026-11-16)

The persistent ref store now records the mint-time `Page.getFrameTree`
`loaderId` for each ref (a `loaderIds` map in `~/.htrcli/refs.json`). The field
is optional: a file written by an older build loads with identity "unknown",
which keeps the conservative pre-fix behaviour.

Consequence: `wait --selector "@eN" --gone --cdp` now PASSES for (a) a
same-document prune and (b) a settled cross-document navigation, while blank
and still-loading pages still FAIL. Live-verified on Chrome 157.

Remaining known limitation: refs are still global rather than scoped to a CDP
target, so a `wait` aimed at a different tab than the mint can read as gone.
Pre-existing, not fixed.

### Number fidelity — settled as opt-in

`htrcli extract table|list --normalize-numbers` is implemented. Without the
flag the output is byte-identical to before. With it:

- U+200D zero-width joiners are removed.
- A `<sup>` renders as `^N` only when its text is all digits AND it directly
  follows `10`, `×10`, or `x10`. So `10<sup>23</sup>` becomes `10^23`, while
  `1<sup>st</sup>` keeps its normal `1st` rendering.
- Footnote markers are removed STRUCTURALLY — an `<a>`, `<sup>` or
  footnote-class element inside the cell — never by regex. A literal `[2]` in
  prose survives, and a footnote-ish `<sup>` wins over the exponent rule.

Two real-page examples from B14:

| Raw `textContent` | With `--normalize-numbers` |
|---|---|
| `"299792458\u200d[2]"` (zero-width joiner + footnote anchor) | `"299792458"` |
| `"6.02214076×10<sup>23</sup>"`, flattened raw to `"6.02214076×1023"` | `"6.02214076×10^23"` |

The CLI sends `options.normalizeNumbers: true` in the command payload; both
`extractTable` and `extractList` honour it.

### Session reuse — corrected record

Named browser contexts already existed: `htrcli context list`, the persistent
`--context` flag, and one isolated persistent `--user-data-dir` per context
(`htrcli/internal/cdp/context.go`). What was missing was bench-level pinning,
now solved by the scenario `context` field. A persisted profile is only
*AUTHENTICATED* after a human logs in once; the harness cannot create that
session.

### Built offline proofs (S24–S28)

S24–S27 pass against a real Chrome on CDP 9555 through the CDP transport:
S24/B21 (32 commands), S25/B22 (16), S26/B23 (15), S27/B24 (11). The
B12/B12a Slack pair (S28, 5 + 5 commands) is prepared but unexecuted:
`requires_auth` reports `NOT_RUN`, never PASS. The fixture table below records
what each proves.

**Live addition (same day)**: S29/B22 (`scenarios/s29-b22-live.json`,
`requires_network`) ran PASS read-only against three Wikipedia pages — 6
commands, 2.09 s — with no login and no write. S31/B23
(`scenarios/s31-b23-live.json`) then ran PASS read-only as a live comparison: the
same constant (`299792458`) is present on both the *Speed of light* and *Metre*
articles, and the second heading is asserted NOT to contain the first, so the two
sources are genuinely distinct — 6 commands, 0.9 s.

**Judged proof (S30)**: `scenarios/s30-judge-demo.json` carries a two-item rubric
and runs through `make bench-judge`, which requires `JUDGE_URL`, `JUDGE_MODEL`
and `JUDGE_AUTH` and fails fast when they are absent. Against a local
Qwen3-0.6B it scored 100 and passed.

Fixtures are addressed through env vars (`BENCH_FIXTURE_CART`,
`BENCH_FIXTURE_RETRIEVAL_1..3`, `BENCH_FIXTURE_COMPARE_A`/`_B`,
`BENCH_FIXTURE_CHALLENGE`) expanded by the Make targets. Running a scenario
JSON directly without those vars leaves a literal `${...}` and fails.

### Judge validation — what is proven and what is not

PROVEN end to end over real HTTP: `judges/openai-judge.sh` is env-only
(requires `JUDGE_URL`, `JUDGE_MODEL`, `JUDGE_AUTH`; `JUDGE_API_TOKEN` only when
`JUDGE_AUTH=bearer`, and the token reaches curl through a config file so it
never appears in argv). Run against a stub OpenAI-compatible endpoint:

- valid scores → PASS with a weighted **86.67** from score 0.9 (weight 2) and
  score 0.8 (weight 1): `(2×0.9 + 1×0.8) / 3 × 100`;
- an out-of-range score of 2 → **judge error**, the scenario FAILS, and no score
  of 0 is invented.

MEASURED with a real local model (2026-11-16): `Qwen3-0.6B-Q8_0.gguf`
(610 MB) served by llama.cpp `llama-server` 2.65.1 on `127.0.0.1:8089` with no
auth (`JUDGE_AUTH=none`) and temperature 0.

- Full run: verdict **PASS**, weighted score **100** (rubric `evidence`
  weight 2 → 1.0; `honesty` weight 1 → 1.0).
- Partial run, with one OPTIONAL step's heading assertion failing: weighted
  **50**, **FAIL** at the default threshold of 60 (`evidence` 0.5,
  `honesty` 0.5). The threshold therefore discriminates rather than
  rubber-stamping.
- A first attempt was rejected with
  `judge returned unknown rubric id "JUDGE-DEMO"`: the tiny model echoed the
  scenario id instead of the rubric ids. That is the runner refusing a bad
  reply, not a score. Fix: the adapter prompt
  (`docs/benchmarks/judges/openai-judge.sh`) now enumerates the ONLY legal ids
  and trims each step's evidence to 400 characters.

Honest caveat: a 0.6B model's reasons are shallow and templated ("passed 2 of
3 steps"). Treat it as a smoke-grade local judge that proves the pipeline and
the weighting, NOT a trustworthy grader. No larger model has been measured.

### Honest limits

- Judge verdict quality is only smoke-grade. The contract and the arithmetic
  are proven against a stub endpoint, and a real local `Qwen3-0.6B-Q8_0` graded
  a full run and a partial run (see "Judge validation"), but a 0.6B model is
  not a trustworthy grader and no larger model has been measured.
- Refs are still global, not scoped to a CDP target, so a `wait --gone` aimed
  at a different tab than the mint can read as gone (see "Ref document
  identity").
- B02–B20 remain `NOT_RUN`: no authenticated sessions exist. Slack and the demo
  tenants are prepared as `requires_auth` scenarios plus a runbook, not as
  executed runs.
- The upstream browser-use task text stays off-limits: it is encrypted, and
  decrypted task text or rubrics must not be republished or used for training.
  The source constraint above is unchanged.

## Comparison with other harnesses

### What is comparable, and what is not

- **Browser Use** publishes per-run aggregates (`official_results/*.json`): tasks,
  successes, total steps, total duration, total cost. Those reduce to a per-task
  shape and are quoted below verbatim.
- **Claude for Chrome** and **ChatGPT's browser agent** publish no per-task
  steps, duration or cost. Their public claims are success rates on third-party
  suites under their own task sets, which cannot be dropped into this table
  without inventing numbers — so their cells read "not published".
- **htrcli is not an agent.** It executes deterministic steps chosen by a driver
  (this session, or a model loop). Its numbers measure harness overhead per task,
  not competence. Only the shape is comparable: steps/task, seconds/task, success
  rate, and cost where a run recorded one.

Provenance for the Browser Use column: `official_results/*.json` in
`browser-use/benchmark` at HEAD `77b4df3`. Each row is one 100-task run; cost is
that run's total divided by the tasks it completed.

| Run | tasks | success | steps/task | seconds/task | cost/task |
|---|---|---|---|---|---|
| bu-v4-opus-4-8 (Cloud API v4) | 100 | 85 | 29.9 | 218 | $1.121 |
| bu-v4-luna (Cloud API v4) | 100 | 78 | 46.4 | 245 | $0.063 |
| Browser Use 0.13.7 + claude-opus-4-7 | 100 | 74 | 19.8 | 478 | not recorded ($0.000) |
| Browser Use 0.13.7 + gpt-5.6-luna | 100 | 31 | 7.1 | 68 | not recorded |
| Browser Use 0.13.7 + gpt-5.5 | 100 | 66 | 17.6 | 371 | not recorded |
| Browser Use 0.13.7 + qwen3.6-plus | 100 | 45 | 36.5 | 1130 | not recorded |

Our runs today, in the same columns, measured by `htrcli bench run` (the report
fields are `commands` and `wall_ms`):

| Scenario | commands | seconds | verdict |
|---|---|---|---|
| S24 cart (offline fixture) | 32 | 0.83 | PASS |
| S25 retrieval (3 offline pages) | 16 | 0.28 | PASS |
| S26 comparison (2 offline pages) | 15 | 0.24 | PASS |
| S27 challenge detect/refuse | 11 | 0.18 | PASS |
| B22 live, 3 Wikipedia pages | 6 | 2.09 | PASS |

### Other harnesses

| Harness | task set | success | steps/task | seconds/task | cost/task |
|---|---|---|---|---|---|
| Claude for Chrome | not published | not published | not published | not published | not published |
| ChatGPT browser agent | not published | not published | not published | not published | not published |
| htrcli (this repo) | our own B-series, S-fixtures | scenario pass rate | `commands` | `wall_ms` | `—` |

A like-for-like comparison needs one task set driven through each harness with
the same judge and the same recorded fields. Browser Use's tasks are encrypted,
so the shared set would have to be public — which is exactly what B21–B24 build
(cart-to-summary, three-source retrieval, two-markup comparison, challenge
detection). Until such a run exists, claim the shape, never a win.

These numbers are NOT a score against their benchmark. The task sets differ:
their runs are 100 tasks; ours are four offline fixtures plus one live scenario.
There is also no LLM in our loop — the driver is this session's tool calls, not a
model choosing actions. So our figures measure harness overhead per task, not
agent competence. Only the SHAPE is comparable: steps/task, seconds/task,
success rate, and cost where a run recorded one. Our token column stays `—`
until a driver supplies `usage`.

## Running the benchmark

The harness is `htrcli bench run <scenario-file>`. A scenario file is JSON
(see `scenarios/`):

```json
{
  "scenarios": [
    {
      "id": "B01",
      "requires_auth": true,
      "steps": [
        {
          "note": "open the inbox",
          "argv": ["open", "https://mail.google.com/mail/u/0/"],
          "expect": { "exit": 0 },
          "retry": 2
        }
      ]
    }
  ]
}
```

- `argv` is a raw htrcli invocation (without the leading `htrcli`).
- `expect` may assert `exit` (default 0), `stdoutContains`,
  `stdoutNotContains` and `jsonPath` (`{"path": "rows.0.Name",
  "equals": "Alice"}` or `{"exists": true}`).
- `retry: N` prefixes `retry --times N --` to the argv, so the retry policy
  lives in one place.
- `optional: true` downgrades a failing step to PARTIAL instead of FAIL.
- `requires_auth: true` is reported as **NOT_RUN** — never as PASS.
- `${VAR}` in an argv is replaced with that environment variable, so a
  scenario file stays machine-independent. Expectation strings
  (`stdoutContains`, `stdoutNotContains`, `jsonPath.equals`) expand `${VAR}`
  too, which is how a scenario asserts on a fixture's absolute `file://` URL
  without hard-coding the machine.
- `capture` pulls values out of a step's stdout into run-scoped variables:
  `"capture": {"revenue": "items.0.q4"}` (dotted JSON path, same syntax as
  `jsonPath`). Later steps in the SAME scenario use `${revenue}`. Captures are
  per-scenario, only taken from a step whose expectations held, never
  re-scanned (a value containing `${...}` passes through literally), a capture
  shadows an environment variable of the same name, and a path that does not
  resolve FAILS the step rather than seeding later steps with an empty string.
  Captures expand inside EXPECTATION strings too (`stdoutContains`,
  `stdoutNotContains`, `jsonPath.equals`), so a scenario can assert that a
  record opened on the id it just captured (B17). Without this, a task that
  carries a fact from one page into another (B14) could only be faked with
  hard-coded text.

Verdicts: PASS (all steps met their expectations), PARTIAL (only optional
steps failed), FAIL (a required step failed), NOT_RUN (skipped), TIMEOUT (a
required step exceeded its deadline), EXCLUDED (skipped by `--exclusions`).
None of NOT_RUN, TIMEOUT or EXCLUDED is ever a PASS.

The report is written to `--out` (default
`docs/benchmarks/results/<timestamp>.json`) and holds the per-step exit code,
attempts, wall time and captured output, plus the per-scenario
`success`/`commands`/`wall_ms`/`retries`/`failure_mode` metrics this document
defines.

### Offline fixture scenarios

These need no authenticated session and touch NO real data: they drive the
local fixture pages in `fixtures/`, which mimic the DOM shapes of the real
apps (a virtualized mail list, a contenteditable reply box with a signature,
a datepicker + guest combobox, a contenteditable doc surface, a cell grid,
a three-step survey wizard whose answers live in the page's model).

| Scenario | File | Covers |
|---|---|---|
| S01 | `scenarios/s01-offline-smoke.json` | open → wait → fill → read-back → extract table → extract list |
| S02, S03 | `scenarios/s02-gmail-fixture.json` | B01 find emails (collect + extract); B02 reply (type appends, signature survives) |
| S04–S06 | `scenarios/s03-office-fixtures.json` | B03 event (datepicker + guest combobox); B05 doc edit; B06 cell entry + grid read-back |
| S07, S08 | `scenarios/s04-tasks-fixture.json` | B04 quick-add (task + date in one field, committed with Enter); B04 full form (popover date grid); B04 ARIA checkbox |
| S09 | `scenarios/s09-form-fixture.json` | B13 multi-page form (validation, select, pick, confirmation ref) |
| S10 | `scenarios/s10-booking-fixture.json` | B07 room select, popover date grid, seat map, confirmation number |
| S11 | `scenarios/s11-expense-fixture.json` | B09 multi-line expense report: dynamic add-line, native select, receipt upload, total + reference capture |
| S12 | `scenarios/s12-files-fixture.json` | B10 upload (name + real size), click-based folder move, share dialog, share-link readback, permission grant, non-destructive check |
| S13 | `scenarios/s13-mail-actions.json` | B08 forward with typed note, custom label popup via `check`, archive proven with `wait --gone` |
| S14 | `scenarios/s14-crm-fixture.json` | B11 filtered contact search (`wait --gone` + `clear`), phone/note/tag-chip edit, save + reopen persistence, activity log attribution |
| S15 | `scenarios/s15-messaging-fixture.json` | B12 react (state survives re-render), contenteditable send via Enter, DM post via the Send button, search with `--gone`, channel isolation both ways |
| S16 | `scenarios/s16-research-fixture.json` | B14 capture two pages' figures + opaque refs into `${var}`s, compose a sourced summary, reject unsourced saves, `stdoutNotContains` the wrong column |
| S17 | `scenarios/s17-thread-replies.json` | B12a reply into a thread from a second contenteditable, per-parent counts (m1=0 vs m2=2), channel history untouched (`count` + `stdoutNotContains`), count survives a channel round trip, empty thread via `:empty` |
| S18 | `scenarios/s18-ticket-fixture.json` | B17 create with two-stage validation + attachment, capture the generated id and reuse it as a selector, refuse Resolve-without-assignee, update status+assignee, comment, seeded tickets untouched |
| S19 | `scenarios/s19-leave-fixture.json` | B19 inclusive weekend-spanning day count (7, not 6 or 5), live duration, inverted range refused, over-balance refused with nothing written, balance 14 → 7 → 14 across a request and a cancellation, both proven by `reload` |
| S20 | `scenarios/s20-social-fixture.json` | B15 hard 280 cap proven by an ACCEPTED 280-char post and refused 281/300, live counter, fixed demo clock making past/now/future deterministic, scheduled vs published states surviving `reload`, reschedule in place without duplication |
| S21 | `scenarios/s21-survey-fixture.json` | B20 three-page survey: per-question and per-row gates, Back restoring radio/select from the model, multi-select add and remove, 10-character boundary (9 refused, 10 accepted), second submission refused, confirmation + response list survive `reload` |
| S22 | `scenarios/s22-finance-fixture.json` | B18 read-only statement: current vs available balance, period filter moving rows/total but never the balance, running balance reconciling with the header, account-scoped reads, and a live access log proving NO write (zero transfer entries, `data-writes` 0) across a `reload` |
| S23 | `scenarios/s23-store-fixture.json` | B16 re-order: quantities carried over while prices come from today's catalogue (one line disagrees), a discontinued line shown but excluded from subtotal and review, shipping included in the grand total, sequential address-then-payment gates, a NEW order id captured (source order untouched), zero 1-click and zero new-card use, and a two-step cancel that survives a `reload` |
| S24 | `scenarios/s24-cart.json` | B21 cart → order summary: two products, one quantity change, SAVE10 = $16.10, total $144.90, payment block never revealed (32 commands) |
| S25 | `scenarios/s25-retrieval.json` | B22 three-page retrieval: page 2 contradicts pages 1 and 3, both values and source ids captured (16 commands) |
| S26 | `scenarios/s26-compare.json` | B23 join across table and list markup: the field absent on B is reported empty, not invented (15 commands) |
| S27 | `scenarios/s27-challenge.json` | B24 challenge: detection and refusal asserted, nothing behind the challenge extracted (11 commands) |
| S28 | `scenarios/s28-slack.json` | B12/B12a Slack: `requires_auth`, reports NOT_RUN, never PASS (5 + 5 commands) |

```bash
make bench-smoke      # S01
make bench-office     # S02–S06
make bench-tasks      # S07–S08
make bench-form       # S09
make bench-booking    # S10
make bench-expense    # S11
make bench-files      # S12
make bench-mail       # S13
make bench-crm        # S14
make bench-messaging  # S15 + S17
make bench-research   # S16
make bench-tickets    # S18
make bench-leave      # S19
make bench-social     # S20
make bench-survey     # S21
make bench-finance    # S22
make bench-store      # S23
make bench-cart       # S24
make bench-retrieval  # S25
make bench-compare    # S26
make bench-challenge  # S27
make bench-slack      # S28 (NOT_RUN offline)
make bench-judge      # S30 (needs JUDGE_URL/JUDGE_MODEL/JUDGE_AUTH)
# Live, read-only, needs network: s29-b22-live.json, s31-b23-live.json
#   ./htrcli/bin/htrcli bench run docs/benchmarks/scenarios/<file> \
#     --transport cdp --allow-network
```

Every target needs a Chrome on the CDP port (9555 by default in
`~/.htrcli/config.json`).

### Safety when running the real scenarios

The B01–B20 live runs are still pending an authenticated session. When they
run:

- Use a **throwaway Google account**. If you are not certain a session is a
  test/staging account, **stop and ask** — do not run it.
- Never send an email to a real person; reply to yourself or save a draft.
- Never invite a real attendee; use a test address.
- Never share a document with anyone but the test account.
- Never enter payment details (B07) or solve a CAPTCHA.
- Never type into a cell or field of a production sheet.

#### What the live runs need

No password is needed by the harness. Each run needs a session that is already
logged in inside a dedicated context, because a context owns its own persistent
`--user-data-dir`:

```bash
htrcli --context live-google open https://mail.google.com   # log in once, by hand
htrcli --context live-slack  open https://app.slack.com
```

The login then survives across runs and touches no other profile. A 2FA or
CAPTCHA prompt must be cleared by the account owner at that login; the harness
never solves one. Tell the driver the context name and confirm the account is
throwaway or staging — writes are attempted only where they are explicitly
allowed.

| Runs | What has to exist | Tasks |
|---|---|---|
| B02, B03, B04, B05, B06, B08, B10 | one throwaway Google account (Gmail, Calendar, Tasks, Docs, Sheets, Drive) | 7 |
| B12, B12a | one free Slack workspace (or a Teams/Discord test server) | 2 |
| B18 | a sandbox or demo banking portal, read-only — never a real bank login | 1 |
| B07, B09, B11, B13, B15, B16, B17, B19, B20 | a demo tenant you own, with writes allowed per task: booking, expense, CRM, form, social, store, helpdesk, HR, survey | 9 |

That is 19 tasks across the 18 blocked entries (B12/B12a share one entry). The
other six tasks are already covered: B01 and B14 by live read-only runs, B21–B24
by their offline proofs plus the B22/B23 live rows.

A Google account plus a Slack workspace unlocks 9 of the 19 and needs no vendor
sign-up beyond those two. The nine demo-tenant tasks each need a separate
sign-up that cannot be automated (email verification), which is why they stay
`NOT_RUN` until someone creates them.


## Notes

- Prefer clicking over constructing URLs (skill rule 3).
- Read values back after every fill; submit once.
- Record baseline metrics with gws/MCP tools for comparison.