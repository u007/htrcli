---
type: Gotcha
title: Bearer token printed to stdout via fmt.Printf, bypassing log multi-writer
description: 'Security: Bearer token was printed to stdout via fmt.Printf at serve.go:72 instead of the structured logger, leaking into system/journal logs regardless of log destination. RESOLVED in the desktop-tray work: now logged as a fingerprint via log.Printf.'
tags:
  - security
  - tray
  - token
  - stdout-leak
  - logging
timestamp: 2026-07-18T14:15:15Z
---
## Gotcha: Bearer token printed to stdout via `fmt.Printf`

**Status: FIXED (2026-07-18, desktop-tray implementation).** `serve.go` no
longer prints the full token. The startup line now uses `log.Printf` and a
`tokenFingerprint` helper that emits only `first4…last4` (or `—` for an
empty/short token), so the value never reaches stdout or the journal. The
`Restart` path additionally strips `--token`/secret flags from the re-exec
argv (see `gotchas/tray-restart-token-leak.md`).

**Severity**: Medium — bearer token leaks into stdout / system logs.

### What happens

The bearer token is printed directly to standard output using `fmt.Printf` instead of being routed through the structured logger's multi-writer:

```go
// htrcli/internal/commands/serve.go:72
fmt.Printf("[htrcli serve] Using bearer token: %s\n", bearerToken)
```

Because `fmt.Printf` writes to `os.Stdout` directly, it bypasses:

- The log multi-writer (which may write to a file, journald, or stderr).
- Log-level filters.
- Any redaction or scrubbing logic applied to the log path.

### Why it's a problem

- On Linux, stdout is commonly piped to `journald`, `syslog`, or a file. The token ends up persisted in plaintext.
- CI/CD pipelines, containers, and daemon managers capture stdout.
- `fmt.Printf` output is not controllable via log-level configuration.

### Fix / Resolution

Replace `fmt.Printf` calls that contain the token with a structured logger call, and never log the full token value:

```go
// Before (serve.go:72):
fmt.Printf("[htrcli serve] Using bearer token: %s\n", bearerToken)

// After:
if bearerToken != "" {
    prefix := bearerToken
    if len(prefix) > 4 {
        prefix = prefix[:4] + "…"
    }
    log.Printf("[htrcli serve] Using bearer token: %s (fingerprint: %s)", bearerToken, prefix)
}
```

Key rules:

1. **Replace `fmt.Printf` with `log.Printf` (or `slog`)**: route through the configured logger so output is controllable via log-level config and goes to the multi-writer.
2. **Never log the full token value**: log only a fingerprint (first 4 chars + ellipsis) for debugging correlation.
3. **Audit all `fmt.Print*` calls** in `serve.go` (lines 66–73) and throughout the tray for other potential credential leaks. The surrounding info lines (listening address, socket path) are safe but should be converted to `log.Printf` for consistency.

### Affected code

- **`htrcli/internal/commands/serve.go:72`** — `fmt.Printf("[htrcli serve] Using bearer token: %s\n", bearerToken)`
- **`htrcli/internal/commands/serve.go:66-67`** — Low-risk info lines that should also be migrated for log-routing consistency.

### Related

- `gotchas/tray-restart-token-leak.md` — same token also leaked via argv.
- `superpowers/reviews/2026-07-18-htrcli-desktop-tray-correctness.md` — correctness review that identified this issue.
