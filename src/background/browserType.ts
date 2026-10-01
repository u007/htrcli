import type { BrowserType } from "../types/commands";

/**
 * Browser capability detection, in its own module.
 *
 * This lives apart from nativeHost.ts because both that module and
 * recordingCommands.ts need it, and nativeHost.ts already imports
 * recordingCommands.ts — importing back the other way would create a cycle. There
 * is still exactly ONE place in the extension that decides which browser it is;
 * nativeHost.ts re-exports these for callers that already import it.
 */

/**
 * Returns the browser capability needed by commands that have different
 * implementations across the shared Chrome/Firefox extension build.
 * Firefox does not expose chrome.debugger; unknown or legacy tab metadata is
 * handled safely by consumers as Chrome-compatible.
 */
export function getBrowserType(): BrowserType {
	return typeof chrome.debugger === "undefined" ? "firefox" : "chrome";
}
