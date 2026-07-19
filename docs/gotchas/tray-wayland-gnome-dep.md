---
type: Gotcha
title: GNOME/Wayland requires gnome-shell-extension-appindicator or ayatana-indicator for systray icon
description: 'Cross-platform: On GNOME/Wayland, the systray icon is invisible unless the user installs a third-party indicator extension or ayatana-indicator.'
tags:
  - platform
  - linux
  - wayland
  - gnome
  - tray
  - dependency
timestamp: 2026-07-18T14:13:12Z
---
## Gotcha: GNOME/Wayland requires appindicator extension for systray icon

**Severity**: High (functional) — tray icon silently disappears; no error is surfaced.

**What happens**: The tray library (`getlantern/systray` or `aymanbagabas/go-systray`) uses the X11 `StatusNotifierItem` / `SystemTray` protocol internally. On GNOME under Wayland, this protocol is **not supported natively** — GNOME removed appindicator support in GNOME 3.26+.

Without one of the following, `systray.Run()` succeeds silently but no icon appears anywhere:

1. **`gnome-shell-extension-appindicator`** — community extension that re-enables appindicator support.
2. **`ayatana-indicator`** — the Ayatana Indicator project (fork of the Unity indicator system), which provides the `StatusNotifierItem` host.

**How to reproduce**:

1. Run the tray binary on a stock GNOME/Wayland session (e.g., Fedora 38+, Ubuntu 22.04+ GNOME).
2. Observe: binary starts, no errors, no tray icon.
3. Install `gnome-shell-extension-appindicator` (available in most distro repos).
4. Restart GNOME Shell (Alt+F2, `r`).
5. Icon appears.

**Fix/Workaround**:

- In documentation/README, document the GNOME/Wayland requirement and link to the extension.
- At startup, detect GNOME under Wayland (`$XDG_SESSION_TYPE == "wayland"` and `$XDG_CURRENT_DESKTOP =~ /GNOME/`) and log a clear warning directing the user to install the extension.
- Consider embedding a notification if no indicator host is detected after a short timeout.

**Affected environments**:

- GNOME Shell on Wayland (default since GNOME 42).
- Vanilla GNOME without custom extensions.
- KDE Plasma (X11/Wayland) works natively — has its own `StatusNotifierItem` host.
- Sway / i3 / other wlroots-based compositors require `status-notifier-watcher` or similar.
