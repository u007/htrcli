/**
 * Connection Manager for the How-To Recorder extension.
 * Auto-detects native messaging vs WebSocket and provides a unified interface.
 */

import {
	connectToServer,
	disconnectFromServer,
	setTabId,
	isConnected as wsIsConnected,
} from "./wsClient";

type ConnectionMode = "native" | "ws" | "disconnected";

let mode: ConnectionMode = "disconnected";

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

chrome.runtime.onMessage.addListener((message) => {
	if (message?.type === "CONNECTION_STATUS") {
		if (message.mode === "native" && mode !== "native") {
			mode = "native";
			if (wsIsConnected()) disconnectFromServer();
			console.log("[ConnectionManager] Switched to native messaging");
			// Re-announce so the daemon's tab registry gets this tab (native port
			// was just (re)connected, so the background will forward it reliably).
			chrome.runtime
				.sendMessage({ type: "CONTENT_SCRIPT_READY", url: window.location.href })
				.catch(() => {});
		} else if (message.mode === "unavailable" && mode === "native") {
			mode = "disconnected";
			checkAutoConnectWS();
		}
	}
});
