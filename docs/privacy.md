# Privacy Policy — HTR NControl

**Last updated:** July 10, 2026

## Overview

HTR NControl ("the Extension") is an open-source browser extension that records user interactions with web pages to generate step-by-step documentation. This privacy policy explains what data the Extension collects, how it is stored, and your rights regarding that data.

**The Extension does not collect, transmit, or sell any data to third parties.** All data stays on your device unless you explicitly export it.

---

## Data We Collect

The Extension captures the following data **locally on your device** while you are actively recording a session:

| Data Type | Description | When Collected |
|-----------|-------------|----------------|
| **Screenshots** | PNG images of the visible browser tab | At recording start, on each interaction, and on user request |
| **Click events** | Element tag, text content, CSS selector, and attributes | When you click interactive elements (buttons, links, inputs, etc.) |
| **Input events** | Form field values, labels, and selectors | When you type in form fields (sensitive values are automatically masked) |
| **Navigation events** | Page URL and title | When you navigate to a new page |
| **Audio narration** | WebM audio recordings (optional) | When you enable audio recording and speak during a step |
| **Annotations** | Text notes you add between steps | When you manually insert an annotation |
| **Tab metadata** | Tab IDs, titles, and URLs | Throughout the recording session |

### What We Do NOT Collect

- Browsing history outside of active recording sessions
- Personal identity information
- Cookies or tracking data
- Analytics, telemetry, or usage statistics
- Data from pages you visit when the Extension is idle
- Form field values from sensitive fields (these are automatically masked — see [Sensitive Data Handling](#sensitive-data-handling) below)

---

## How Your Data Is Stored

All recorded data is stored **locally in your browser** using:

- **IndexedDB** (`HowToRecorderDB`) — Primary storage for screenshots, steps, annotations, and audio blobs
- **chrome.storage.local** — Extension settings and configuration

**No data is sent to any remote server, cloud service, or third party.** Your recordings exist only in your browser's local storage.

---

## Sensitive Data Handling

The Extension automatically detects and masks sensitive form field values before they are stored. This includes:

- **Passwords** (`<input type="password">`)
- **Credit card numbers** (fields with `autocomplete="cc-number"`, `cc-csc`, etc.)
- **Social Security Numbers** and other government identifiers
- **API keys, tokens, and secrets** (fields with names/labels matching common patterns like `api_key`, `token`, `secret`, `ssn`, `otp`, `2fa`, etc.)

Detected sensitive values are replaced with `********` at the time of capture. The Extension sets a `isSensitive` flag on these steps, and exports display `"[Sensitive data masked]"` in place of the actual value.

---

## Data Transmission

The Extension may communicate over the network **only** in the following limited, local scenarios:

### Local WebSocket Server (Remote Control)

When remote control is enabled, the Extension connects to a WebSocket server running on your own machine (`ws://127.0.0.1:3845`). This server is:

- Bound to localhost only (not accessible from the network)
- Protected by IP whitelist and optional bearer token authentication
- Used solely for executing remote commands (click, fill, navigate, screenshot) from your own tools

### Native Messaging (htcli)

The Extension can optionally connect to a local native messaging host (`com.htrcontrol.host`) for remote control. This communication stays entirely on your device.

### No External Connections

The Extension makes **zero** connections to external servers, APIs, or third-party services. There is:

- No analytics or tracking
- No telemetry
- No crash reporting
- No advertising networks
- No data brokers

---

## Permissions

The Extension requests the following browser permissions:

| Permission | Purpose |
|------------|---------|
| `activeTab` | Capture screenshots of the current tab |
| `tabs` | Track tab navigation and manage multiple tabs |
| `contextMenus` | Provide right-click annotation options |
| `downloads` | Save exported files (JSON, Markdown, ZIP) to your device |
| `storage` | Store extension settings locally |
| `scripting` | Inject content scripts into newly opened tabs |
| `sidePanel` | Display the main user interface |
| `nativeMessaging` | Connect to the local htcli native host (optional) |
| `debugger` | Generate PDFs via Chrome DevTools Protocol (optional) |
| `host_permissions: ["<all_urls>"]` | Inject content scripts to track interactions on any page you visit while recording |

**Note:** The `host_permissions` permission is required so the Extension can capture your interactions on any website. Content scripts are only active when you are recording a session.

---

## Data Export

You can export your recorded sessions at any time using the Export feature in the side panel. Available formats:

- **JSON** — Machine-readable session data
- **Markdown** — Human-readable step-by-step documentation with embedded screenshots
- **ZIP** — Bundled archive containing JSON, Markdown, screenshots, and audio files

Exports are triggered **manually by you**. The Extension never automatically exports or uploads your data.

---

## Data Retention

Your recorded sessions remain in your browser's IndexedDB until you:

- Manually delete them through the Extension's UI
- Clear your browser's extension data
- Uninstall the Extension

The Extension does not automatically delete or expire your data.

---

## Children's Privacy

The Extension is not directed at children under 13. We do not knowingly collect information from children.

---

## Changes to This Policy

We may update this privacy policy from time to time. Changes will be reflected in the "Last updated" date at the top of this document. Continued use of the Extension after changes constitutes acceptance of the updated policy.

---

## Open Source

HTR NControl is open-source software. You can review the complete source code to verify all claims made in this privacy policy:

- **Repository:** [github.com/jameshowe/how-to-recorder](https://github.com/jameshowe/how-to-recorder)
- **Key files:**
  - `src/contentScript/inputHandler.ts` — Sensitive field masking logic
  - `src/utils/sensitiveFields.ts` — Sensitive field detection patterns
  - `src/background/index.ts` — Screenshot capture and data storage
  - `src/manifest.ts` — Extension permissions

---

## Contact

If you have questions about this privacy policy or the Extension's data practices, please open an issue on our [GitHub repository](https://github.com/jameshowe/how-to-recorder/issues).
