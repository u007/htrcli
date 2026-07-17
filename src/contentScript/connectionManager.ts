/**
 * Connection Manager for the HTR NControl extension.
 * Auto-detects native messaging vs WebSocket and provides a unified interface.
 */

import {
	connectToServer,
	disconnectFromServer,
	setConnectionChangeCallback,
	setTabId,
	isConnected as wsIsConnected,
} from "./wsClient";

type ConnectionMode = "native" | "ws" | "disconnected" | "unavailable";

let mode: ConnectionMode = "disconnected";
// Tracks whether we've reported a connected WS to the background, so we only
// send status transitions (not every reconnect attempt).
let reportedWs = false;

// Report the WebSocket connection state to the background, which folds it
// into the unified connection mode shown in the side panel. We only report
// "ws" once the socket is actually open and "disconnected" once it drops,
// so a flapping socket doesn't spam status messages.
function reportWsStatus(connected: boolean): void {
	if (connected && !reportedWs) {
		reportedWs = true;
		chrome.runtime
			.sendMessage({ type: "WS_CONNECTION_STATUS", mode: "ws" })
			.catch(() => {});
	} else if (!connected && reportedWs) {
		reportedWs = false;
		chrome.runtime
			.sendMessage({ type: "WS_CONNECTION_STATUS", mode: "disconnected" })
			.catch(() => {});
	}
}

setConnectionChangeCallback(reportWsStatus);

// ─── Init ───────────────────────────────────────────────────────────

export async function connect(): Promise<void> {
	// Fetch real tab ID from background
	const tabId = await getRealTabId();
	if (tabId) setTabId(tabId);

	// Ask background for native host status
	const status = await getConnectionStatus();

	if (status === "native") {
		mode = "native";
		console.log("[ConnectionManager] Using native messaging");
		return;
	}

	// Native unavailable — fall back to WebSocket
	mode = "ws";
	console.log(
		"[ConnectionManager] Native unavailable, falling back to WebSocket",
	);
	await checkAutoConnectWS();
}

export function disconnect(): void {
	if (mode === "ws") {
		disconnectFromServer();
	}
	mode = "disconnected";
}

export function isConnected(): boolean {
	if (mode === "native") return true;
	if (mode === "ws") return wsIsConnected();
	return false;
}

// ─── Helpers ────────────────────────────────────────────────────────

function getRealTabId(): Promise<number | null> {
	return new Promise((resolve) => {
		chrome.runtime.sendMessage({ type: "GET_TAB_ID" }, (resp) => {
			if (chrome.runtime.lastError || !resp?.tabId) {
				resolve(null);
				return;
			}
			resolve(resp.tabId as number);
		});
	});
}

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

async function checkAutoConnectWS(): Promise<void> {
	try {
		const result = await chrome.storage.local.get([
			"remoteControlServer",
			"remoteControlToken",
		]);
		if (result.remoteControlServer) {
			const token = result.remoteControlToken as string | undefined;
			const url = token
				? `${result.remoteControlServer as string}?token=${encodeURIComponent(token)}`
				: (result.remoteControlServer as string);
			connectToServer(url);
		}
	} catch {
		// storage unavailable
	}
}

// ─── Listen for status changes from background ────────────────────
//
// This listener and `setConnectionChangeCallback(reportWsStatus)` in the
// init section are two complementary channels that both keep `mode` in
// sync, but cover different directions:
//
//   - `setConnectionChangeCallback` (line ~39) is the LIVE channel: the
//     WebSocket client pushes open/close transitions UP to the background
//     via `WS_CONNECTION_STATUS` messages. The background uses them to
//     compute the unified connection mode for the side panel.
//   - This `chrome.runtime.onMessage` listener is the PUSHED channel: the
//     background broadcasts `CONNECTION_STATUS` DOWN when its own
//     `nativeHostMode` changes (daemon connect/disconnect, etc.). The
//     handler reacts to those transitions by switching between native and
//     the WebSocket auto-connect fallback.
//
// The two paths are not strictly redundant: the callback only knows
// about THIS tab's WS state, while the listener knows about the
// background's view of the native host. They meet in `mode` so the rest
// of the content script can use one value.
chrome.runtime.onMessage.addListener((message) => {
	if (message?.type === "CONNECTION_STATUS") {
		if (message.mode === "native") {
			mode = "native";
			if (wsIsConnected()) disconnectFromServer();
			console.log("[ConnectionManager] Switched to native messaging");
			// Re-announce so the daemon's tab registry gets this tab (native port
			// was just (re)connected, so the background will forward it reliably).
			chrome.runtime
				.sendMessage({
					type: "CONTENT_SCRIPT_READY",
					url: window.location.href,
				})
				.catch(() => {});
			return;
		}

		// Any non-native mode (ws / disconnected / unavailable) means native
		// messaging is not the active transport. If we were previously on
		// native, switch to the WebSocket fallback so the tab stays connected.
		if (mode === "native") {
			mode = "disconnected";
			checkAutoConnectWS();
			return;
		}

		// The background's own view of `wsConnected` lives in memory and is
		// wiped whenever its MV3 service worker is evicted and restarts (it
		// re-runs the whole script, resetting the flag to false). Our socket
		// survives that restart untouched, but `reportedWs` is already true so
		// `reportWsStatus` won't re-send. If the background just broadcast a
		// non-native status while our socket is actually open, re-announce it
		// so the background (and side panel) catch back up.
		if (wsIsConnected()) {
			reportedWs = true;
			chrome.runtime
				.sendMessage({ type: "WS_CONNECTION_STATUS", mode: "ws" })
				.catch(() => {});
		}
	}
});
