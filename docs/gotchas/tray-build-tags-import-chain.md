---
type: Gotcha
title: Build tags must be on ALL files referencing getlantern/systray to break the C import chain on unsupported OSes
description: 'Architecture: Build tags (e.g., //go:build !linux) must be placed on every file that imports getlantern/systray, not just the entry point, or the CGo import chain will fail to compile on unsupported OSes.'
tags:
  - architecture
  - build
  - cgo
  - systray
  - build-tags
  - cross-platform
timestamp: 2026-07-18T14:13:12Z
---
## Gotcha: Build tags must be on ALL files referencing `getlantern/systray` to break the C import chain

**Severity**: High (build failure) — cross-platform compilation silently fails with confusing CGo errors.

**What happens**: `getlantern/systray` uses CGo internally — it links against platform-specific system libraries (Win32 API, macOS Cocoa/AppKit, Linux X11/GTK). If you guard only the entry point (e.g., `systray.Run()`) with a build tag but leave supporting files that also import the package unguarded, the Go compiler will:

1. See the import of `getlantern/systray` in an unguarded file.
2. Try to compile the C source files vendored by the library.
3. Fail with errors like:

```
# github.com/getlantern/systray
exec: "gcc": executable file not found in $PATH
```

or on Windows targeting Linux:

```
cc1: error: unrecognized command line option "-m64"
```

**The rule**: Every `.go` file that imports `getlantern/systray` (directly or transitively) must carry a build constraint that matches the target OS:

```go
//go:build linux || darwin || windows

package tray

import (
    "github.com/getlantern/systray"
)
```

For files that are only needed on specific platforms, use narrower constraints:

```go
// Platform-specific systray implementation.
//go:build linux

package tray
```

**Common pitfalls**:

- **Transitive imports**: A file in package `foo` imports `github.com/getlantern/systray`. Package `bar` imports package `foo` without any build tags. Now `bar` also needs the constraint because Go transitively resolves all imports.
- **Untagged test files**: `systray_test.go` or `foo_test.go` may import the package. Tests must also carry the constraint or they'll fail on `go test ./...` on unsupported OSes.
- **Init-time registration**: If a package uses `init()` to register a systray driver, that file must be tagged. Otherwise the compiler tries to compile the CGo dependency on every OS.

**Detection**:

Run `go build ./...` on an unsupported OS (or via GOOS/GOARCH cross-compile) to find unguarded files. The error is always a C toolchain failure, not a Go type error.

```bash
GOOS=freebsd go build ./...   # Will fail if any file imports systray without a tag
```

On macOS targeting Linux:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build ./...
```

**Best practice**:

- Keep all systray-related code in a single package with one file per OS, each carrying `//go:build <os>`.
- Use a build-tag-free interface/abstraction that the tagged code implements, so the rest of the app never imports `getlantern/systray` directly.
- Add a CI step that cross-compiles the package for all unsupported platforms to catch build-tag drift early.
