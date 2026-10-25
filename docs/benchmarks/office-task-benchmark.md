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

- `success`: PASS / PARTIAL / FAIL
- `commands`: number of htrcli invocations
- `wall_ms`: wall-clock milliseconds
- `retries`: number of retries / recoveries
- `failure_mode`: short description if not PASS

Scoring: PASS = all pass/fail checks met on first attempt. PARTIAL = completed
with 1 retry/recovery. FAIL = not completed or >2 retries.

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
- **Likely htrcli gaps**: RESOLVED — `wait` (DOM condition) and `collect`
  (virtualized list) both exist; `extract list` turns rows into
  sender/subject/date records. Verified against the local Gmail fixture
  (scenario S02).
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
  survives a reload — which needs `reload` over `--cdp`, the transport every
  scenario uses. That last one was a hard stop: `reload` used to answer
  "reload is not supported over --cdp yet", so no scenario could prove
  persistence at all. It is now implemented (Page.reload + wait for the load
  event) and this scenario is its first user. The date fields are native
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
steps failed), FAIL (a required step failed), NOT_RUN (skipped).

The report is written to `--out` (default
`docs/benchmarks/results/<timestamp>.json`) and holds the per-step exit code,
attempts, wall time and captured output, plus the per-scenario
`success`/`commands`/`wall_ms`/`retries`/`failure_mode` metrics this document
defines.

### Offline fixture scenarios

These need no authenticated session and touch NO real data: they drive the
local fixture pages in `fixtures/`, which mimic the DOM shapes of the real
apps (a virtualized mail list, a contenteditable reply box with a signature,
a datepicker + guest combobox, a contenteditable doc surface, a cell grid).

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
```

Both need a Chrome on the CDP port (9555 by default in
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

## Notes

- Prefer clicking over constructing URLs (skill rule 3).
- Read values back after every fill; submit once.
- Record baseline metrics with gws/MCP tools for comparison.