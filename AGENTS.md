# AGENTS.md - How-To Recorder

Guidelines for AI coding agents working in this Chrome extension codebase.

## Project Overview

How-To Recorder is a browser extension that records user interactions (clicks, inputs, navigation) with screenshots and optional audio narration, then exports them as step-by-step documentation. The same source code ships as a **Chrome extension** (MV3) and a **Firefox extension** (MV3 with `sidebar_action`); see `firefox/` for the Firefox build.

### Architecture

- **Background Service Worker** (`src/background/`) - Orchestrates recording, captures screenshots, manages state
- **Content Scripts** (`src/contentScript/`) - Injected into pages to track clicks/inputs
- **Side Panel** (`src/sidepanel/`) - React UI for controlling recordings and viewing timeline (shared by both Chrome and Firefox)
- **Types** (`src/types/`) - Shared TypeScript interfaces and message types
- **Utils** (`src/utils/`) - Export functions (JSON, Markdown, ZIP), sensitive field detection

### Cross-browser layout

- The Chrome build uses `@crxjs/vite-plugin` with `src/manifest.ts` (MV3, `side_panel`).
- The Firefox build lives under `firefox/` and uses plain Vite (the crxjs plugin is Chrome-only) with a small `manifest` emitter built into `firefox/vite.config.ts`. It shares every source file in `src/` and adds only thin entry shims that import `webextension-polyfill` and apply a Firefox-specific `chrome.sidePanel` stub (see `firefox/src/firefox-shims.ts`).
- The polyfill is loaded first in every entry, so the shared `chrome.*` calls in `src/` resolve to Firefox's native `browser.*` API at runtime without any source modifications.
- See `firefox/README.md` for the full architecture, build commands, and XPI packaging.

## Package Manager

Use `bun` for all package management operations:

```bash
bun install          # Install dependencies
bun add <package>    # Add dependency
bun add -D <package> # Add dev dependency
bun remove <package> # Remove dependency
```

## Build Commands

```bash
bun run build    # TypeScript check + Vite production build (outputs to build/)
bun run dev      # Start Vite dev server with HMR
bun run zip      # Build and create distributable ZIP
bun run preview  # Preview production build

# Firefox (separate workspace under firefox/)
bun run firefox:build      # tsc -p firefox/tsconfig.json + vite build
bun run firefox:typecheck  # tsc -p firefox/tsconfig.json --noEmit
bun run firefox:dev        # Vite dev server for Firefox
bun run firefox:zip        # Build and create firefox/build/...xpi
```

## Linting & Formatting

This project uses **Biome** for linting and formatting:

```bash
bun run biome check .              # Check formatting and lint rules
bun run biome check --write .      # Auto-fix issues
bun run biome format .             # Format only
bun run biome lint .               # Lint only
bun run biome check src/utils/     # Check specific directory
bun run biome check src/file.ts    # Check single file
```

## Code Style Guidelines

### Formatting (Biome)

- **Indentation**: Tabs
- **Quotes**: Double quotes for JS/TS strings
- **Imports**: Auto-organized by Biome

### TypeScript

- **Strict mode enabled** - No implicit any, strict null checks
- **Target**: ESNext
- **Module**: ESNext with Node resolution
- **JSX**: react-jsx (automatic runtime)

### Naming Conventions

| Type             | Convention                                     | Example                                      |
| ---------------- | ---------------------------------------------- | -------------------------------------------- |
| Interfaces       | PascalCase                                     | `RecordingSession`, `ElementInfo`            |
| Types            | PascalCase                                     | `MessageType`, `TimelineItem`                |
| Functions        | camelCase                                      | `startRecording`, `getElementText`           |
| Constants        | UPPER_SNAKE_CASE                               | `SENSITIVE_INPUT_TYPES`, `INPUT_DEBOUNCE_MS` |
| React Components | PascalCase                                     | `ExportPanel`, `Timeline`                    |
| CSS Classes      | kebab-case                                     | `export-panel`, `timeline-item`              |
| Files            | camelCase for utils, PascalCase for components | `exportMarkdown.ts`, `ExportPanel.tsx`       |

### Import Organization

Order imports as follows (Biome auto-organizes):

1. React and external libraries
2. Type imports (use `import type` for types only)
3. Internal modules by path depth
4. CSS imports last

```typescript
import React, { useState, useCallback } from 'react'
import type { RecordingSession, RecordingStep } from '../../types/recording'
import { useRecording } from '../context/RecordingContext'
import './ExportPanel.css'
```

### Type Definitions

- Use `interface` for object shapes that may be extended
- Use `type` for unions, intersections, and aliases
- Export types from `src/types/recording.ts` for shared use
- Prefer explicit return types on exported functions

```typescript
// Interface for extensible objects
export interface ElementInfo {
  tag: string
  text: string
  selector: string
}

// Type for unions
export type MessageType = 'START_RECORDING' | 'STOP_RECORDING' | 'CLICK_EVENT'

// Explicit return types
export function generateMarkdown(session: RecordingSession): string {
  // ...
}
```

### Error Handling

- Use try/catch for async Chrome API calls
- Log errors with `console.error` or `console.warn` with `[How-To Recorder]` prefix
- Gracefully handle extension context invalidation (extension reload)

```typescript
try {
  const response = await chrome.runtime.sendMessage({ type: 'GET_STATE' })
} catch (error) {
  console.warn('[How-To Recorder] Failed to send message:', error)
}
```

### React Patterns

- Use functional components with hooks
- Use `useReducer` for complex state (see `RecordingContext.tsx`)
- Use `useCallback` for functions passed to children or used in effects
- Wrap context hooks with validation

```typescript
export function useRecording(): RecordingContextType {
  const context = useContext(RecordingContext)
  if (!context) {
    throw new Error('useRecording must be used within a RecordingProvider')
  }
  return context
}
```

### Chrome Extension Patterns

- Content script to background: `chrome.runtime.sendMessage()`
- Background to content script: `chrome.tabs.sendMessage(tabId, message)`
- Always return `true` from message listeners for async responses
- Use `{ capture: true }` for event listeners to catch events early

```typescript
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  handleAsync().then(() => sendResponse({ success: true }))
  return true // Required for async response
})
```

## Project-Specific Conventions

### Message Types

All messages between components use typed interfaces from `src/types/recording.ts`:

- Define message type in `MessageType` union
- Create corresponding interface extending `BaseMessage`
- Add to `RecordingMessage` union type

### Sensitive Data

Fields containing passwords, credit cards, etc. are automatically detected and masked:

- Detection logic in `src/utils/sensitiveFields.ts`
- Values displayed as `********` in recordings

### Screenshots

- Captured via `chrome.tabs.captureVisibleTab()`
- Stored as base64 PNG data URLs
- Elements highlighted before capture (red border overlay)

## Cross-Browser Development Workflow

**When updating anything in the extension, always work on both Chrome and Firefox.** The shared `src/` code affects both browsers, but each has its own build pipeline and runtime quirks.

### Rule: Always update both browsers

1. **Shared code (`src/`)** — Changes here apply to both browsers automatically. After making changes, verify both builds pass:
   ```bash
   bun run build          # Chrome build
   bun run firefox:build  # Firefox build
   ```

2. **Chrome-specific code (`src/manifest.ts`)** — If you change Chrome manifest permissions, entry points, or MV3 config, check whether Firefox needs an equivalent change in `firefox/vite.config.ts` (which emits the Firefox manifest).

3. **Firefox-specific code (`firefox/`)** — If you add a shim or Firefox-only workaround in `firefox/src/`, consider whether the same fix should be applied to the shared `src/` code so both browsers benefit. Document why a Firefox-only path exists.

4. **Server changes (`server/`)** — The server (port 3845) is shared by both browsers via WebSocket. After changing server endpoints, message handling, or auth:
   - Verify the extension's content script (`src/contentScript/connectionManager.ts`) still connects correctly
   - Test with both Chrome and Firefox if possible, or at minimum confirm no browser-specific assumptions were introduced in the server code

### Verification checklist

After any extension or server change, run these commands before committing:

```bash
bun run check:fix         # Lint + format
bun run build             # Chrome build
bun run firefox:build     # Firefox build
bun run test              # Unit tests (if any)
```

If the change touches server code or the WebSocket connection layer:

```bash
bun run server:dev        # Start server locally
# Manually verify extension connects in both Chrome and Firefox
```

### Why this matters

- The Firefox build uses `webextension-polyfill` to alias `chrome.*` → `browser.*`, so Chrome-only API calls may silently fail on Firefox if not shimmed.
- The server doesn't know which browser is talking to it — both connect over the same WebSocket protocol. A breaking change in the server protocol breaks both browsers equally.

## File Structure

```
src/
├── background/       # Service worker
├── contentScript/    # Injected scripts (click, input, highlight handlers)
├── sidepanel/        # React UI
│   ├── components/   # UI components
│   └── context/      # React context providers
├── types/            # TypeScript definitions
├── utils/            # Export and utility functions
├── db/               # IndexedDB schema (for future use)
└── manifest.ts       # Extension manifest configuration
```

## Testing

No test framework is currently configured. When adding tests:

- Use Bun's built-in test runner: `bun test`
- Run single test: `bun test src/path/to/file.test.ts`
