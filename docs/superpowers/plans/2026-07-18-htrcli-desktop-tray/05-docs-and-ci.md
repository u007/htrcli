# Part 5: Documentation, CI, and Manual Smoke Tests

The final part. Documentation tells users what the tray does and how to disable it. CI must opt out so runners don't try to start a tray. Manual smoke tests catch the cross-platform issues that automated tests can't see.

---

### Task 13: New `htrcli/docs/tray.md` (user-facing reference)

**Files:**
- Create: `htrcli/docs/tray.md`

**Content:**

- [ ] **Step 1: Write the doc**

```markdown
# htrcli tray icon

When you run `htrcli serve` on a desktop, a small icon appears in your
menu bar (macOS) or system tray (Windows, Linux). Click it for live
status, lifecycle controls, and common maintenance tasks.

## What it does

The tray menu has two read-only labels and a Maintenance submenu:

- **Status** — port, number of connected browsers, and a quick
  `ok` / `error` indicator.
- **Last error** — the most recent non-fatal error from the daemon
  (cleared after 5 minutes or on the next success).
- **Maintenance**:
  - **Reinstall native host** — re-register the browser extension's
    native messaging host in Chrome or Firefox. Use this after the
    extension gets a new ID.
  - **Open config folder** — opens `~/.htrcli/` in Finder/Explorer/
    your file manager.
  - **Copy bearer token** — copies the bearer token to the clipboard
    for 30 seconds, then overwrites it with `<cleared by htrcli>`.
  - **Show recent log** — opens `~/.htrcli/serve.log` in your
    default app.
  - **Restart** — cleanly restart the daemon (re-applies config).
  - **Quit** — cleanly shut down the daemon.

## Headless behavior

The tray is automatically skipped when:
- No `DISPLAY` is set (X11)
- No `WAYLAND_DISPLAY` is set (Wayland)
- You're logged in over SSH (`SSH_CONNECTION` or `SSH_TTY` is set)
- You pass `--no-tray` or set `HTRCLI_NO_TRAY=1`

On a headless Linux server, `htrcli serve` prints one info line
(`Tray disabled (no display or HTRCLI_NO_TRAY set)`) and continues
as a pure daemon. Server operators see no difference.

## Opting out

Even on a desktop, you can disable the tray:

```bash
htrcli serve --no-tray
HTRCLI_NO_TRAY=1 htrcli serve
```

Use this in CI, systemd units, and Docker containers.

## Per-platform notes

| Platform | Tray | Notes |
|---|---|---|
| macOS | Always shown | Cmd-Q on the icon exits cleanly. |
| Windows | Always shown | Right-click → Quit exits. |
| Linux (X11) | Shown if `DISPLAY` is set | Works on KDE, XFCE, MATE, Cinnamon, etc. |
| Linux (Wayland) | Shown if `WAYLAND_DISPLAY` is set | **GNOME 42+ requires `gnome-shell-extension-appindicator` or `ayatana-indicator` to be installed.** KDE Plasma 6 has native support. |
| Headless Linux | Skipped | See "Headless behavior" above. |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| No tray icon on Linux | GNOME/Wayland missing the indicator extension | Install `gnome-shell-extension-appindicator` |
| "Reinstall native host" disabled | No extension ID configured | Run `htrcli config set-extension-id <id> --browser chrome` (or firefox) |
| "Copy bearer token" returns no output on Wayland | `xclip` doesn't work on Wayland | Install `wl-clipboard` (`wl-copy`); the tray detects `WAYLAND_DISPLAY` and uses it automatically |
| "Show recent log" says "No log yet" | Daemon just started; log file is empty | Wait a few seconds, click again |
| Token in clipboard is permanent | macOS clipboard manager retained it | Disable the clipboard manager, or don't use "Copy bearer token" — read it from the log file or config instead |
```

- [ ] **Step 2: Verify it renders**

```bash
cat htrcli/docs/tray.md
```

(Skipped in CI; just visual review for v1.)

- [ ] **Step 3: Commit**

```bash
cd htrcli && git add docs/tray.md
git commit -m "docs(htrcli): tray.md user-facing reference

Covers what the menu does, headless behavior, opt-out flag,
per-platform notes, and troubleshooting.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 14: Update README, SPEC, CHANGELOG, and htrcli skill

**Files:**
- Modify: `htrcli/README.md` (add a "Tray icon" subsection under "Daemon")
- Modify: `htrcli/SPEC_HTRCLI.md` (add a "Tray icon" section at the end)
- Modify: `CHANGELOG.md` (add user-visible entry)
- Modify: `skills/htrcli/SKILL.md` (add a paragraph under "Setup")

- [ ] **Step 1: Update `htrcli/README.md`**

In the "Daemon" section (find it; should be near the top), add a "Tray icon" subsection:

```markdown
### Tray icon

When you run `htrcli serve` on a desktop (macOS, Windows, Linux with a
display), a system-tray icon appears automatically. See
[htrcli/docs/tray.md](docs/tray.md) for what the menu does and how to
disable it.

Headless Linux servers (no display, or logged in over SSH) silently
skip the tray — no configuration needed.
```

- [ ] **Step 2: Update `htrcli/SPEC_HTRCLI.md`**

At the end (after the existing sections on daemon, HTTP API, relay, etc.), add:

```markdown
## Tray icon

`htrcli serve` auto-attaches a cross-platform system-tray icon on
desktops. The menu exposes live status and a small set of maintenance
actions (reinstall native host, open config folder, copy bearer token,
show recent log, restart, quit). The tray is silently skipped on
headless Linux; opt out with `--no-tray` or `HTRCLI_NO_TRAY=1`.

See `docs/tray.md` for the user-facing reference and
`docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md` for
the full design rationale.
```

- [ ] **Step 3: Update `CHANGELOG.md`**

Add at the top under the next unreleased version (create the version
header if there isn't one):

```markdown
## [Unreleased]

### Added

- **Tray icon (desktop)**: `htrcli serve` now shows a cross-platform
  system-tray icon on macOS, Windows, and Linux desktops. Menu provides
  live status (port, relay count, last error) and maintenance actions:
  reinstall native host (Chrome/Firefox), open config folder, copy
  bearer token (with 30s auto-clear), show recent log, and Restart/Quit
  lifecycle. The main goroutine now drives the tray; the HTTP server and
  signal handler run as goroutines behind it. On headless Linux servers
  the tray is silently skipped; opt out with `--no-tray` or
  `HTRCLI_NO_TRAY=1` (CI must use this). Bearer token is now logged as
  a fingerprint (`a1b2…f3e4`) instead of the full value. Requires
  `gnome-shell-extension-appindicator` or `ayatana-indicator` on
  GNOME/Wayland for the icon to appear.
```

- [ ] **Step 4: Update `skills/htrcli/SKILL.md`**

In the "Setup" section, after the existing `htrcli serve` description,
add a one-paragraph note:

```markdown
### Tray icon

When you run `htrcli serve` on a desktop, a system-tray icon auto-attaches.
It exposes live status and maintenance actions (reinstall native host,
open config folder, copy bearer token, show recent log, restart, quit).
On headless Linux servers (no display, or SSH session), the tray is
silently skipped. See `htrcli/docs/tray.md` for the full menu and
`--no-tray` opt-out.
```

- [ ] **Step 5: Verify all four files render and the new content is consistent**

```bash
git diff htrcli/README.md htrcli/SPEC_HTRCLI.md CHANGELOG.md skills/htrcli/SKILL.md | head -100
```

- [ ] **Step 6: Commit**

```bash
git add htrcli/README.md htrcli/SPEC_HTRCLI.md CHANGELOG.md skills/htrcli/SKILL.md
git commit -m "docs: document htrcli tray icon in README, SPEC, CHANGELOG, skill

Single-line summary in README, full spec section in SPEC_HTRCLI.md,
user-visible changelog entry, and a setup note in the htrcli skill.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 15: CI opt-out enforcement

CI invocations of `htrcli serve` MUST pass `--no-tray`. The `Makefile` may have a target that starts the daemon; the GitHub Actions workflow may run integration tests. Audit and update.

**Files:**
- Modify: `Makefile` (any target that runs `htrcli serve` for a sustained period)
- Modify: `.github/workflows/*.yml` (if any workflow runs `htrcli serve`)
- Modify: `htrcli/Makefile` (the `make htrcli-build` target may not invoke `serve`, but check)

- [ ] **Step 1: Find all `htrcli serve` invocations**

```bash
cd /Users/james/www/how-to-recorder && grep -rn "htrcli serve" --include="*.yml" --include="Makefile" --include="*.mk" --include="*.sh" --include="*.bash"
```

Expected: at most a handful of hits. Each must be updated to include `--no-tray`.

- [ ] **Step 2: Update each invocation**

Pattern:
```bash
# before
htrcli serve &
# after
htrcli serve --no-tray &
```

For systemd unit files (if any), set `Environment="HTRCLI_NO_TRAY=1"`.

- [ ] **Step 3: Run the full test suite end-to-end**

```bash
cd htrcli && go test ./...
cd htrcli && go test -tags=traytest ./internal/tray/ -v
cd htrcli && go vet ./...
cd htrcli && go build ./...
```

Expected: all green.

- [ ] **Step 4: Commit**

```bash
git add Makefile .github/ htrcli/Makefile
git commit -m "ci: opt out of tray in all htrcli serve invocations

macOS and Windows CI runners (and Linux headless) must not attempt
to start a tray. Add --no-tray / HTRCLI_NO_TRAY=1 to every serve
invocation in CI configs and Makefile targets.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 16: Manual smoke tests (the parts that can't be automated)

The cross-platform UI behavior can only be verified by running on real desktops. Do these on each target platform before merging. Document results in a `htrcli/docs/tray-smoke-results.md` file (or in the PR description).

**Files:**
- Create: `htrcli/docs/tray-smoke-results.md` (per-platform results)

- [ ] **Step 1: macOS smoke test**

```bash
cd htrcli && make htrcli-build
./bin/htrcli serve
```

Verify:
- Icon appears in the menu bar.
- Menu opens; "Status: 3845 · 0 relays · ok" is shown.
- "Maintenance → Open config folder" opens `~/.htrcli/` in Finder.
- "Show recent log" opens `~/.htrcli/serve.log` in Console (or default app).
- "Copy bearer token" places the token on the clipboard; 30s later, the clipboard is `<cleared by htrcli>`.
- Click "Quit" → clean exit (verify with `ps aux | grep htrcli`).
- In another terminal: `kill -TERM <pid>` → clean exit (no zombie, no error).

Record results in `tray-smoke-results.md`.

- [ ] **Step 2: Linux X11 smoke test (e.g. KDE Plasma)**

```bash
cd htrcli && make htrcli-build
HTRCLI_SERVER=http://127.0.0.1:3845 ./bin/htrcli serve
```

Verify:
- Icon appears in the system tray.
- "Maintenance → Reinstall native host" is initially disabled (no ext-id configured).
- After `htrcli config set-extension-id <id>`, the menu items become enabled.
- "Copy bearer token" uses `xclip` (verify with `xclip -o | head -c 8`).
- "Quit" exits cleanly.

Record results.

- [ ] **Step 3: Linux Wayland smoke test (GNOME 45+)**

Same as Step 2, but on Wayland.

Verify:
- Icon appears **only** if `gnome-shell-extension-appindicator` is installed. Without it, document the failure in `tray-smoke-results.md` and the troubleshooting section of `tray.md`.
- "Copy bearer token" uses `wl-copy` (verify with `wl-paste | head -c 8`).
- All other menu items work as on X11.

- [ ] **Step 4: Windows smoke test**

```bash
cd htrcli && make htrcli-build
./bin/htrcli.exe serve
```

Verify:
- Icon appears in the system tray.
- Menu items work.
- "Copy bearer token" uses `clip.exe` (verify with a paste in Notepad).
- "Quit" exits cleanly.

- [ ] **Step 5: Headless Linux smoke test**

```bash
# In a headless VM or container:
cd htrcli && make htrcli-build
./bin/htrcli serve
```

Verify:
- One info log line: `Tray disabled (no display or HTRCLI_NO_TRAY set)`.
- No error.
- Exit code 0 if the daemon is later killed with SIGTERM.
- Port 3845 is bound (verify with `ss -lnt | grep 3845` or `lsof -i :3845`).

- [ ] **Step 6: Save results**

```bash
git add htrcli/docs/tray-smoke-results.md
git commit -m "docs(htrcli): tray manual smoke test results

Records per-platform verification: macOS, Linux X11, Linux Wayland,
Windows, headless Linux. Each test result captured in the table.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Part 5 complete when:

- `htrcli/docs/tray.md` exists and covers the user-facing surface.
- `htrcli/README.md`, `htrcli/SPEC_HTRCLI.md`, `CHANGELOG.md`, `skills/htrcli/SKILL.md` all mention the tray.
- All CI invocations of `htrcli serve` pass `--no-tray` or set `HTRCLI_NO_TRAY=1`.
- Manual smoke tests are recorded in `htrcli/docs/tray-smoke-results.md` for each target platform.

## Final verification

After all 5 parts are committed, run the full validation from `AGENTS.md` / `CLAUDE.md`:

```bash
# From the repo root:
bun run check:fix                # Biome (no-op for Go changes, but harmless)
cd htrcli && go test ./...       # all Go tests
cd htrcli && go test -tags=traytest ./internal/tray/ -v   # full menu simulation
cd htrcli && go build ./...      # all three platforms cross-compile
cd htrcli && go vet ./...        # static analysis
```

The feature is ready to merge when:
- All Go tests pass (regular and `traytest`).
- `go build` succeeds on darwin, linux, windows.
- `go vet` reports no issues.
- The 5-platform smoke test table is filled in.
- CHANGELOG and docs are accurate.

The brainstorming flow is now complete: spec → plan → ready to implement.
