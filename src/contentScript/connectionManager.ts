/**
 * Connection Manager for the HTR NControl extension.
 * Auto-detects native messaging and provides a unified interface.
 */
import type { ConnectionMode } from "../types/recording";

let mode: ConnectionMode = "disconnected";

// ─── Init ───────────────────────────────────────────────────────────

export async function connect(): Promise<void> {
	// Ask background for native host status
	const status = await getConnectionStatus();

	if (status === "native") {
		mode = "native";
		console.log("[ConnectionManager] Using native messaging");
		return;
	}

	// Native unavailable
	mode = "unavailable";
	console.log("[ConnectionManager] Native messaging unavailable");
}

export function disconnect(): void {
	mode = "disconnected";
}

export function isConnected(): boolean {
	return mode === "native";
}

// ─── Helpers ────────────────────────────────────────────────────────

function getConnectionStatus(): Promise<"native" | "unavailable"> {
	return new Promise((resolve) => {
		chrome.runtime.sendMessage({ type: "GET_CONNECTION_STATUS" }, (resp) => {
			if (chrome.runtime.lastError || resp?.mode !== "native") {
				resolve("unavailable");
				return;
			}
			resolve("native");
		});
	});
}

// ─── Listen for status changes from background ────────────────────

chrome.runtime.onMessage.addListener((message) => {
	if (message?.type === "CONNECTION_STATUS") {
		if (message.mode === "native") {
			mode = "native";
			console.log("[ConnectionManager] Switched to native messaging");
			// Re-announce so the daemon's tab registry gets this tab (native port
			// was just (re)connected, so the background will forward it reliably).
			chrome.runtime
				.sendMessage({
					type: "CONTENT_SCRIPT_READY",
					url: window.location.href,
				})
				.catch(() => {});
		} else {
			// native disconnected or unavailable
			mode = message.mode;
		}
	}
});
