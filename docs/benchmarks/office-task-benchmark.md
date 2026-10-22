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

### B07 — Booking (meeting room / travel / appointment) — P1
- **App**: booking site
- **Goal**: Book a resource for a time slot; capture confirmation number.
- **Start state**: search form available.
- **Pass/fail**: confirmation number captured; booking appears in account.
- **Likely gaps**: date pickers, seat maps; payment gating (never enter card).
- **Safety**: use a clearly-labelled test slot; cancel after capture.

### B08 — Forward / label / archive email — P1
- **App**: Gmail web
- **Goal**: Forward a thread with a note; apply label; archive.
- **Pass/fail**: forwarded in Sent; label applied; thread archived.

### B09 — Expense report / invoice submission — P2
- **App**: expense portal
- **Goal**: Fill expense form with line items (date, category, amount, receipt
  upload), submit.
- **Pass/fail**: submission confirmation; totals match.
- **Likely gaps**: receipt upload, multi-page form.

### B10 — File management (Drive/OneDrive/Dropbox) — P2
- **App**: cloud drive web
- **Goal**: Upload a file, move to folder, share link, set permissions.
- **Pass/fail**: file in target folder; share link works.

### B11 — Contact / CRM update — P2
- **App**: CRM web
- **Goal**: Find contact, update fields (phone, note, tag), log activity.
- **Pass/fail**: fields persisted; activity logged.

### B12 — Messaging (Slack/Teams/Discord web) — P2
- **App**: messaging web
- **Goal**: Post a message to a channel/DM, react, search history.
- **Pass/fail**: message visible; search returns it.

### B13 — Form filling (HR/IT/surveys) — P2
- **App**: web form
- **Goal**: Fill multi-page form end-to-end, submit, capture ref.
- **Pass/fail**: all fields match; submission ref captured.

### B14 — Research & summarize (web) — P2
- **App**: web pages
- **Goal**: Open N pages, extract key facts, produce a summary.
- **Pass/fail**: summary cites sources; no hallucinated facts.

### B15 — Social media post / schedule — P3
- **App**: social platform web
- **Goal**: Compose and (optionally) schedule a post with media.
- **Pass/fail**: post visible on profile / scheduled in queue.
- **Likely gaps**: rich-text/media upload, scheduling date pickers.

### B16 — E-commerce reorder — P3
- **App**: store account web
- **Goal**: Reorder a previous purchase; capture order number.
- **Pass/fail**: order confirmation number captured.
- **Safety**: use test store / cancel if possible; never enter real card.

### B17 — Support ticket create / update — P3
- **App**: helpdesk web
- **Goal**: Create ticket with subject/body/attachment; add a comment.
- **Pass/fail**: ticket number captured; comment visible.

### B18 — Bank / finance portal read-only check — P3
- **App**: bank web
- **Goal**: Read balance and recent transactions; no writes.
- **Pass/fail**: balance + ≥1 transaction captured.
- **Safety**: read-only; never initiate transfers.

### B19 — HR leave request — P3
- **App**: HR portal
- **Goal**: Submit a leave request with dates and reason.
- **Pass/fail**: request number/status captured.

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
  scenario file stays machine-independent.

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

```bash
make bench-smoke      # S01
make bench-office     # S02–S06
make bench-tasks      # S07–S08
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