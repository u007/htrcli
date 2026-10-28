# htrcli headless / CDP gaps — plan index (2026-10-28)

Source: an external evaluation of `htrcli` in a container listed six gaps
(no Chrome launch flags, no viewport emulation, no color-scheme emulation,
no raw CDP escape hatch, `screenshot --annotate` extension-only, opaque launch
failure). Source review confirmed all six and added: macOS-only Chrome
discovery, no dialog handling over `--cdp`, `scroll`/`back`/`forward`/
`printpdf` rejected over `--cdp`, launch args not recorded in state.

Key architectural fact driving the design: every `--cdp` invocation opens a
fresh CDP session and closes it on return (`internal/commands/cdp_exec.go`
`cdpSession`, callers `defer s.Close()`). CDP `Emulation.*` overrides are
session-scoped, so a raw passthrough cannot deliver persistent emulation.
Emulation therefore gets first-class verbs with persisted state re-applied on
every session open.

Parts (each self-contained; no cross-references needed):

| Part | Scope | Depends on |
|---|---|---|
| `01-launch.md` | `--chrome-arg` / config / env extra Chrome args, stderr log + error tail, cross-platform Chrome discovery, args persisted in browser state | none |
| `02-emulate.md` | `emulate` verb group (viewport, color-scheme, user-agent, show, reset) persisted per CDP port and re-applied in `cdpSession` | none |
| `03-cdp-parity.md` | Port `scroll`, `back`, `forward`, `printpdf`, `dialog handle`, `screenshot --annotate` to `--cdp` | none |
| `04-cdp-raw.md` | New `cdp <method> [params-json]` stateless passthrough verb | none |

Execution order: 01 ‖ 02, then 03 ‖ 04. Parts 03 and 04 both touch
`internal/commands/inspect.go` and `skills/htrcli/SKILL.md`; use targeted edits,
never whole-file rewrites.

Shared conventions for every part:
- Go 1.22, module `github.com/u007/htrcli`. Run `gofmt`, `go vet ./...`,
  `go build ./...`, `go test ./...` from `htrcli/` before reporting.
- Fail fast and loud. No silent fallbacks, no `value || default` style code,
  no empty catch/ignored errors. Log every caught error with what was attempted.
- Touch only what the part names. Match existing style.
- Docs are part of the change: `htrcli/README.md`, `skills/htrcli/SKILL.md`
  (the `--cdp` works / not-available table at ~line 52), and `htrcli/TODO.md`
  where a part resolves or adds a known limitation.
- Do not commit.
