// Firefox-specific shim for Chrome-only APIs.
//
// The shared source code uses `chrome.sidePanel.*` to manage the
// sidebar (e.g. `setOptions` to set a per-tab panel, and
// `setPanelBehavior({ openPanelOnActionClick: true })` to open the
// sidebar when the user clicks the extension's toolbar icon).
// Firefox has no `sidePanel` namespace — its equivalent is
// `sidebarAction`, which is configured via the manifest's
// `sidebar_action.default_panel`.
//
// We patch `chrome.sidePanel` at startup with no-op stubs so the
// existing calls in `src/background/index.ts` don't throw at
// runtime. Firefox's behavior is then driven by the manifest
// (always show the same panel) and by the existing
// `chrome.action.onClicked` listener (which the source already
// registers and which Firefox fires the same way as Chrome).

import "webextension-polyfill";

// The `@types/chrome` definitions in scope are stricter than what
// Firefox exposes; we type the polyfilled globals as `unknown` and
// assert only the subset we actually touch, so we don't need a
// single blanket `any` cast that defeats type-checking elsewhere.
// biome-ignore lint/suspicious/noExplicitAny: shim for cross-browser API
const g = globalThis as any;
// biome-ignore lint/suspicious/noExplicitAny: shim for cross-browser API
const browserApi: any = g.browser ?? g.chrome;
if (browserApi && !g.chrome) {
	g.chrome = browserApi;
}

// If `chrome.sidePanel` is missing, polyfill it with no-op stubs.
// The manifest's `sidebar_action` configuration is what actually
// controls Firefox's sidebar; these stubs exist purely so the
// shared source's `chrome.sidePanel.*` calls don't throw.
if (!browserApi.sidePanel) {
	browserApi.sidePanel = {
		// No-op: Firefox uses the manifest-declared
		// `sidebar_action.default_panel` for every tab; the per-tab
		// `path` parameter from the Chrome API has no equivalent.
		async setOptions(_options: {
			tabId?: number;
			path?: string;
			enabled?: boolean;
		}): Promise<void> {
			// Intentionally empty.
		},
		// No-op: Firefox has no `openPanelOnActionClick` runtime
		// toggle. To make the toolbar click actually open the
		// sidebar in Firefox, we register a one-time
		// `sidebarAction.open()` call on the action click. We do it
		// here so the Firefox build doesn't need to modify the
		// shared source.
		async setPanelBehavior(options: {
			openPanelOnActionClick?: boolean;
		}): Promise<void> {
			if (!options.openPanelOnActionClick) return;
			if (browserApi.sidebarAction?.open && browserApi.action?.onClicked) {
				browserApi.action.onClicked.addListener(() => {
					browserApi.sidebarAction.open().catch(() => {
						// Sidebar may already be open; ignore.
					});
				});
			}
		},
	};
}
