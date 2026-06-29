/**
 * Native Messaging Host integration for the How-To Recorder extension.
 * Manages the connection to the htcli native host via Chrome Native Messaging.
 */

import type { Command, CommandResult, TabInfo } from "../types/commands";

const HOST_NAME = "com.howtorecorder.host";
const RECONNECT_BASE_MS = 1000;
const RECONNECT_MAX_MS = 30000;

let nativePort: chrome.runtime.Port | null = null;
let reconnectDelay = RECONNECT_BASE_MS;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let connectionMode: "native" | "unavailable" = "unavailable";

// ─── Public API ────────────────────────────────────────────────────

export function startNativeHost(): void {
	connect();
}

export function getConnectionMode(): "native" | "unavailable" {
	return connectionMode;
}

export function sendToNative(msg: object): void {
	if (nativePort) {
		try {
			nativePort.postMessage(msg);
		} catch (err) {
			console.error("[NativeHost] postMessage failed:", err);
		}
	}
}

// ─── Connection ────────────────────────────────────────────────────

function connect(): void {
	if (reconnectTimer) {
		clearTimeout(reconnectTimer);
		reconnectTimer = null;
	}

	try {
		nativePort = chrome.runtime.connectNative(HOST_NAME);
	} catch (err) {
		console.warn("[NativeHost] connectNative failed:", err);
		markUnavailable();
		return;
	}

	nativePort.onMessage.addListener(handleNativeMessage);
	nativePort.onDisconnect.addListener(() => {
		const err = chrome.runtime.lastError?.message ?? "unknown";
		console.warn(`[NativeHost] Disconnected: ${err}`);
		nativePort = null;

		if (err.includes("not found") || err.includes("not installed")) {
			markUnavailable();
			return;
		}

		// Relay died (SW was killed) — retry with backoff
		scheduleReconnect();
	});

	connectionMode = "native";
	reconnectDelay = RECONNECT_BASE_MS;
	console.log("[NativeHost] Connected");

	// Broadcast new status to all content scripts
	broadcastStatus();
	// Register any tabs that are already open so the daemon can target them
	// immediately, even if their content scripts loaded before the relay.
	syncRegisteredTabs();
}

function markUnavailable(): void {
	connectionMode = "unavailable";
	nativePort = null;
	broadcastStatus();
}

function scheduleReconnect(): void {
	reconnectTimer = setTimeout(() => {
		reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_MAX_MS);
		connect();
	}, reconnectDelay);
}

function syncRegisteredTabs(): void {
	if (!nativePort) return;

	chrome.tabs.query({}, (tabs) => {
		for (const tab of tabs) {
			if (tab.id == null) continue;
			if (!tab.url || !/^https?:\/\//.test(tab.url)) continue;

			registerTab(tab.id, {
				id: tab.id,
				url: tab.url || "",
				title: tab.title || "",
				active: tab.active || false,
				favIconUrl: tab.favIconUrl,
			});
		}
	});
}

function broadcastStatus(): void {
	chrome.tabs.query({}, (tabs) => {
		for (const tab of tabs) {
			if (tab.id == null) continue;
			chrome.tabs
				.sendMessage(tab.id, {
					type: "CONNECTION_STATUS",
					mode: connectionMode,
				})
				.catch(() => {
					// Content script may not be loaded on this tab — ignore
				});
		}
	});
}

interface NativeCommandMessage {
	type: "command";
	tabId: number;
	payload: Command;
}

interface NativeRegisterAckMessage {
	type: "ping";
}

type NativeMessage = NativeCommandMessage | NativeRegisterAckMessage;

function handleNativeMessage(msg: NativeMessage): void {
	if (msg.type === "command") {
		const { tabId, payload } = msg;
		chrome.tabs.sendMessage(
			tabId,
			{
				type: "EXECUTE_COMMAND",
				command: payload,
			},
			(result: CommandResult) => {
				if (chrome.runtime.lastError) {
					// Tab may be closed; relay error back to daemon
					sendToNative({
						type: "command_result",
						tabId,
						payload: {
							id: payload.id,
							success: false,
							error: "tab not available",
						},
					});
					return;
				}
				sendToNative({
					type: "command_result",
					tabId,
					payload: result,
				});
			},
		);
	}
}

// ─── Tab registration ─────────────────────────────────────────────

export function registerTab(tabId: number, info: TabInfo): void {
	sendToNative({
		type: "register",
		tabId,
		payload: info,
	});
}
