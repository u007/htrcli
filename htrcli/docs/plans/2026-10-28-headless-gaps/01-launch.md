# Part 01 — Chrome launch: extra args, stderr capture, discovery, state

## Problem

`htrcli browser start --headless` aborts in containers that lack user
namespaces because Chrome needs `--no-sandbox` (and often
`--disable-dev-shm-usage`). There is no way to add Chrome flags, Chrome's
stderr is discarded so the failure is opaque, and Chrome is only auto-found at
the macOS application path. The recorded browser state does not say how Chrome
was launched. `TODO.md` already asks for a launch-args hook so integration
tests can pass `--use-mock-keychain`.

## Files

- `internal/cdp/launch.go` — `LaunchArgs`, `launchChrome`, `StartBrowser`,
  `FindChrome`, `BrowserState`.
- `internal/cdp/launch_test.go` — `TestLaunchArgs`, `TestLaunchArgsHeadless`,
  `TestFindChrome*`, `TestStateRoundTrip`, `TestLaunchChromeTimeoutReapsChild`.
- `internal/cdp/context.go` — `EnsureContext` calls `launchChromeFn`; must
  forward extra args.
- `internal/commands/browser.go` — `browser start` flags, `browser status`
  output.
- `internal/commands/root.go` — `GetChromePath`, `resolveContext`, viper env
  wiring (`HTRCLI_` prefix, dash→underscore replacer).
- `internal/commands/config.go` — `set-chrome-path` is the pattern for a new
  `set-chrome-args`.
- `README.md` (sections "CDP transport", "Configuration", "Requirements"),
  `skills/htrcli/SKILL.md` (CDP transport section ~line 187 and config block
  ~line 226), `TODO.md` (the mock-keychain note under "pick / fill-form /
  --frame").

## Design

### Extra Chrome arguments

Three sources, merged in this order: config key `chrome-args` (a JSON string
array in `~/.htrcli/config.json`, also readable from env
`HTRCLI_CHROME_ARGS` as a whitespace-separated string through viper's slice
getter), then the repeatable `--chrome-arg` flag on `browser start`. Flag
values are appended after config values. The resolved list is passed into
`LaunchArgs`, which appends it after its own fixed list.

Add a `config set-chrome-args <arg>...` command following `set-chrome-path`;
zero args clears the key. `config show` prints the list.

Validation happens once, in a single function in `internal/cdp/launch.go`,
before launch. Reject with a clear error any extra arg whose name is
`--remote-debugging-port`, `--remote-debugging-address`, or `--user-data-dir`
(with or without a value). The first two protect the localhost-only binding the
existing security comment on `LaunchArgs` requires; the third would break the
profile check `StopBrowser` relies on. No other filtering.

`LaunchArgs` gains an `extra []string` parameter. `launchChrome`,
`StartBrowser`, and `EnsureContext` all gain the same parameter and thread it
through. `resolveContext` in `root.go` passes the config/env args (named
contexts have no flag of their own; the config list applies to them too).

### Chrome stderr

Chrome runs detached and outlives htrcli, so its stderr must not be a pipe
(a closed pipe would SIGPIPE Chrome later). Instead `launchChrome` opens
`~/.htrcli/chrome-stderr.log` (truncate on each launch, mode 0600) and sets it
as the child's stderr and stdout. On the timeout path and in the reaper's
"Chrome exited" message, read the last ~2 KB of that file and append it to the
message as a `last stderr:` section, trimming to whole lines. The log path is
also printed by `browser status` when the port is not alive, so a failed start
is a one-shot diagnosis. Reading the tail is a helper in `launch.go` with its
own unit test (temp file, longer than the cap, verify whole-line trimming).

### Chrome discovery

`FindChrome` currently tries the configured path then the macOS Chrome path.
Extend to a per-OS candidate list, configured path first:

- darwin: Google Chrome, Google Chrome Canary, Chromium under `/Applications`.
- linux: `google-chrome`, `google-chrome-stable`, `chromium`,
  `chromium-browser` resolved via `exec.LookPath`, then the same names under
  `/usr/bin` and `/snap/bin`.
- windows: Chrome under `Program Files`, `Program Files (x86)`, and
  `LOCALAPPDATA`.

Keep the candidate list builder a separate pure function taking the GOOS
string so it is unit-testable without a real Chrome. The not-found error lists
every path tried and keeps the `config set-chrome-path` hint.

### Persist launch args

Add `Args []string` to `BrowserState` (JSON `args`), set to the full argv
`LaunchArgs` produced. `browser status` prints them in text mode and they
appear in `--json`. `TestStateRoundTrip` covers the new field.

## Tests

- `LaunchArgs` with extra args appends them after the fixed list and after
  `--headless`.
- Validation rejects each forbidden arg name, with and without `=value`, and
  accepts `--no-sandbox`, `--window-size=390,844`, `--force-dark-mode`.
- Candidate builder returns the expected ordered list per GOOS.
- stderr tail helper: cap, whole-line trim, missing file is an error.
- Existing `TestLaunchChromeTimeoutReapsChild` still passes; extend it to assert
  the error message contains the `last stderr:` marker when the fake child
  wrote to stderr.

## Docs

- README: document `--chrome-arg`, `config set-chrome-args`,
  `HTRCLI_CHROME_ARGS`, the stderr log path, and a container recipe
  (`--no-sandbox --disable-dev-shm-usage`). Note Linux/Windows auto-discovery.
- SKILL.md: same, briefly, in the CDP transport section and the config block.
- TODO.md: replace the mock-keychain wrapper workaround with
  `HTRCLI_CHROME_ARGS=--use-mock-keychain` and drop the "needs a test-only
  launch-args hook" sentence.
