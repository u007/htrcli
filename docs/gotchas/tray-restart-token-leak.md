---
type: Gotcha
title: Restart() re-exec leaks bearer token in /proc/*/cmdline
description: 'Security: Restart() re-exec leaked bearer token in /proc/*/cmdline. RESOLVED: stripSecrets() removes --token/--bearer/--password/--api-key and their values from the re-exec argv; the child re-resolves the token from env/file/viper.'
tags:
  - security
  - tray
  - token
  - cmdline-leak
  - linux
timestamp: 2026-07-18T14:15:15Z
---
## Gotcha: Restart() re-exec leaks bearer token in `/proc/*/cmdline`

**Severity**: Medium — local information disclosure on Linux.

### What happens

When the tray daemon calls `Restart()`, the function re-execs itself using `os.Executable()` + `exec.Command`. The bearer token may be passed as a `--token` flag on the command line (depending on how `args` are constructed):

```go
// Hypothetical or planned pattern (from tray design spec):
cmd := exec.Command(exe, "serve", "--token", tokenValue, ...)
```

or more broadly, if any re-exec path conveys the token via argv, it is world-readable on Linux.

On Linux, every process's `argv` is world-readable via `/proc/<pid>/cmdline`. Any user on the system (or any process running as the same user, or root) can read the bearer token from there without special privileges.

### Why it's a problem

- The bearer token authenticates the htrcli ↔ extension native-messaging channel.
- `/proc/*/cmdline` has no access control by default (readable by the process owner).
- Container environments, CI runners, and multi-user workstations amplify the risk.

### Fix / Resolution

**Strip `--token` (and any credential-carrying flags) from the re-exec argv.** The token is already resolved by `serve.go` from `HTR_BEARER_TOKEN` env → token file → viper config, so there is no need to pass it as a CLI flag on restart. The re-exec process will resolve the token from the same sources.

The fix pattern in `Restart()`:

```go
// Before (hypothetical — passes all args including --token):
cmd := exec.Command(selfPath, append([]string{"serve"}, args...)...)

// After — filter out --token and its value from args:
func stripTokenFlag(args []string) []string {
    out := make([]string, 0, len(args))
    for i := 0; i < len(args); i++ {
        if args[i] == "--token" || args[i] == "-t" {
            i++ // skip value too
            continue
        }
        if strings.HasPrefix(args[i], "--token=") {
            continue
        }
        out = append(out, args[i])
    }
    return out
}

cmd := exec.Command(selfPath, append([]string{"serve"}, stripTokenFlag(args)...)...)
```

Key rules:

1. **Never pass credentials on argv** — not just `--token`, but any flag whose value is a secret.
2. **Filter `--token` (and `-t` or `--token=value`) from re-exec args** before spawning the child process.
3. **Alternative**: use `ExtraFiles` (inherited file descriptor), temp file with `0600` permissions, or environment variable — but the simplest correct fix is to simply omit the secret flag from argv since the child will re-resolve it from the same sources.
4. **Audit all re-exec paths** for other potential credential leaks in argv.

### Current code status

**Status: FIXED (2026-07-18, desktop-tray implementation).** `internal/tray/daemon_controller.go`
implements `Restart()` which calls `stripSecrets(os.Args[1:])` before re-execing
`selfPath serve …`. `stripSecrets` drops `--token`, `--bearer`, `--password`,
`--api-key` (and `=value` forms) plus their values, so nothing secret reaches
`/proc/<pid>/cmdline`. The re-exec'd process re-resolves the token from
`HTR_BEARER_TOKEN` (inherited from the parent) or the config file, so behavior
is unchanged. `Restart` also closes the HTTP listener first to avoid the
port-binding race.

### Related

- `gotchas/tray-token-print-stdout.md` — same token also printed to stdout.
- `superpowers/reviews/2026-07-18-htrcli-desktop-tray-correctness.md` — correctness review covering Restart design.
- `superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md` — tray design spec describing Restart re-exec.
