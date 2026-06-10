/**
 * WebSocket Client for connecting the extension to the remote control server.
 * Runs in the content script context to maintain a persistent connection.
 *
 * When the extension connects to the server, it registers all open tabs
 * and listens for commands to execute.
 */

import type {
	Command,
	CommandResult,
	ExtensionMessage,
	ServerMessage,
	TabInfo,
} from "../types/commands";
import { executeCommand } from "./commandExecutor";

// ─── Configuration ─────────────────────────────────────────────────

const DEFAULT_SERVER_URL = "ws://127.0.0.1:3845";
const RECONNECT_DELAY = 3000; // 3 seconds
const MAX_RECONNECT_DELAY = 30000; // 30 seconds
const HEARTBEAT_INTERVAL = 15000; // 15 seconds

// ─── State ─────────────────────────────────────────────────────────

let ws: WebSocket | null = null;
let reconnectDelay = RECONNECT_DELAY;
let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let isConnecting = false;
let serverUrl = DEFAULT_SERVER_URL;

// ─── Public API ────────────────────────────────────────────────────

/**
 * Connect to the remote control server.
 * Call this from the content script to enable remote control.
 */
export function connectToServer(url?: string): void {
	if (url) serverUrl = url;

	if (ws?.readyState === WebSocket.OPEN || isConnecting) {
		return;
	}

	isConnecting = true;
	console.log(`[How-To Recorder] Connecting to server: ${serverUrl}`);

	try {
		ws = new WebSocket(serverUrl);

		ws.onopen = () => {
			console.log("[How-To Recorder] Connected to remote control server");
			isConnecting = false;
			reconnectDelay = RECONNECT_DELAY;

			// Register this tab with the server
			registerTab();

			// Start heartbeat
			startHeartbeat();
		};

		ws.onmessage = (event) => {
			try {
				const message = JSON.parse(event.data) as ServerMessage;
				handleServerMessage(message);
			} catch (error) {
				console.error(
					"[How-To Recorder] Failed to parse server message:",
					error,
				);
			}
		};

		ws.onclose = (event) => {
			console.log(
				`[How-To Recorder] Disconnected from server (code: ${event.code}, reason: ${event.reason})`,
			);
			isConnecting = false;
			stopHeartbeat();
			scheduleReconnect();
		};

		ws.onerror = (error) => {
			console.warn("[How-To Recorder] WebSocket error:", error);
			isConnecting = false;
		};
	} catch (error) {
		console.warn("[How-To Recorder] Failed to connect to server:", error);
		isConnecting = false;
		scheduleReconnect();
	}
}

/**
 * Disconnect from the server.
 */
export function disconnectFromServer(): void {
	if (reconnectTimer) {
		clearTimeout(reconnectTimer);
		reconnectTimer = null;
	}

	stopHeartbeat();

	if (ws) {
		ws.close(1000, "Client disconnect");
		ws = null;
	}
}

/**
 * Check if connected to the server.
 */
export function isConnected(): boolean {
	return ws?.readyState === WebSocket.OPEN;
}

// ─── Message Handling ──────────────────────────────────────────────

function handleServerMessage(message: ServerMessage): void {
	switch (message.type) {
		case "command":
			if (message.command) {
				handleCommand(message.command, message.tabId);
			}
			break;

		case "ping":
			// Respond to ping
			if (ws?.readyState === WebSocket.OPEN) {
				const response: ExtensionMessage = {
					type: "heartbeat",
					tabId: getTabId(),
					timestamp: Date.now(),
				};
				ws.send(JSON.stringify(response));
			}
			break;

		case "disconnect":
			console.log("[How-To Recorder] Server requested disconnect");
			disconnectFromServer();
			break;
	}
}

async function handleCommand(
	command: Command,
	_targetTabId?: number,
): Promise<void> {
	const startTime = Date.now();
	console.log(
		`[How-To Recorder] Executing remote command: ${command.action} (${command.id})`,
	);

	try {
		const result = await executeCommand(command);

		// Capture screenshot if requested
		if (command.options?.screenshot) {
			try {
				// Request screenshot from background
				const screenshotResponse = await chrome.runtime.sendMessage({
					type: "CAPTURE_SCREENSHOT",
					tabId: getTabId(),
				});
				if (screenshotResponse?.screenshotData) {
					result.screenshot = screenshotResponse.screenshotData;
				}
			} catch {
				// Screenshot might fail, that's ok
			}
		}

		// Send result back to server
		sendResult(command.id, result);
	} catch (error) {
		console.error("[How-To Recorder] Command execution failed:", error);
		sendResult(command.id, {
			id: command.id,
			success: false,
			error: error instanceof Error ? error.message : String(error),
			duration: Date.now() - startTime,
		});
	}
}

function sendResult(commandId: string, result: CommandResult): void {
	if (ws?.readyState === WebSocket.OPEN) {
		const message: ExtensionMessage = {
			type: "command_result",
			tabId: getTabId(),
			commandId,
			result,
			timestamp: Date.now(),
		};
		ws.send(JSON.stringify(message));
	}
}

// ─── Tab Registration ──────────────────────────────────────────────

function registerTab(): void {
	if (ws?.readyState !== WebSocket.OPEN) return;

	const tabInfo: TabInfo = {
		id: getTabId(),
		url: window.location.href,
		title: document.title,
		active: document.hasFocus(),
	};

	const message: ExtensionMessage = {
		type: "register",
		tabId: tabInfo.id,
		tabInfo,
		timestamp: Date.now(),
	};

	ws.send(JSON.stringify(message));
}

function getTabId(): number {
	// Content scripts don't have direct access to tab ID
	// We'll use a hash of the URL as a pseudo-ID
	// The background script can provide the real tab ID if needed
	return hashString(window.location.href);
}

function hashString(str: string): number {
	let hash = 0;
	for (let i = 0; i < str.length; i++) {
		const char = str.charCodeAt(i);
		hash = (hash << 5) - hash + char;
		hash |= 0; // Convert to 32-bit integer
	}
	return Math.abs(hash);
}

// ─── Heartbeat ─────────────────────────────────────────────────────

function startHeartbeat(): void {
	stopHeartbeat();
	heartbeatTimer = setInterval(() => {
		if (ws?.readyState === WebSocket.OPEN) {
			const message: ExtensionMessage = {
				type: "heartbeat",
				tabId: getTabId(),
				timestamp: Date.now(),
			};
			ws.send(JSON.stringify(message));
		}
	}, HEARTBEAT_INTERVAL);
}

function stopHeartbeat(): void {
	if (heartbeatTimer) {
		clearInterval(heartbeatTimer);
		heartbeatTimer = null;
	}
}

// ─── Reconnection ──────────────────────────────────────────────────

function scheduleReconnect(): void {
	if (reconnectTimer) return;

	console.log(`[How-To Recorder] Reconnecting in ${reconnectDelay / 1000}s...`);

	reconnectTimer = setTimeout(() => {
		reconnectTimer = null;
		reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY);
		connectToServer();
	}, reconnectDelay);
}

// ─── Auto-connect ──────────────────────────────────────────────────

// Check if remote control is enabled via URL parameter or storage
async function checkAutoConnect(): Promise<void> {
	try {
		// Check storage for server URL
		const result = await chrome.storage.local.get("remoteControlServer");
		if (result.remoteControlServer) {
			connectToServer(result.remoteControlServer);
			return;
		}

		// Check URL parameter
		const urlParams = new URLSearchParams(window.location.search);
		const serverParam = urlParams.get("htr-server");
		if (serverParam) {
			connectToServer(serverParam);
		}
	} catch {
		// Storage might not be available
	}
}

// Auto-connect on load
checkAutoConnect();
