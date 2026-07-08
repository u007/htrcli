/**
 * Content Script - Main Entry Point
 * Handles communication with background script, coordinates
 * click/input tracking, executes remote control commands,
 * and connects to the remote control server via WebSocket.
 */

import type { Command, CommandResult } from "../types/commands";
import type {
	ClickEventMessage,
	EnableRecordingMessage,
	HighlightElementMessage,
	InputEventMessage,
	RecordingMessage,
} from "../types/recording";
import { startClickTracking, stopClickTracking } from "./clickHandler";
import { executeCommand } from "./commandExecutor";
import {
	connect as connectRemote,
	disconnect as disconnectRemote,
	isConnected as remoteIsConnected,
} from "./connectionManager";
import { hideHighlight, removeHighlight, showHighlight } from "./highlighter";
import {
	flushPendingInputs,
	startInputTracking,
	stopInputTracking,
} from "./inputHandler";

console.info("[HTR NControl] Content script loaded");

// ─── Recording State ─────────────────────────────────────────────

let isRecording = false;
let recordingStartTime: number | null = null;

// ─── Remote Control State ─────────────────────────────────────────

let remoteControlEnabled = false;

// ─── Message Helpers ──────────────────────────────────────────────

/**
 * Send a message to the background script
 */
function sendToBackground(message: RecordingMessage): void {
	chrome.runtime.sendMessage(message).catch((error) => {
		// Extension context may be invalidated if extension is reloaded
		console.warn("[HTR NControl] Failed to send message:", error);
	});
}

/**
 * Handle click events from the click handler
 */
function handleClickEvent(message: ClickEventMessage): void {
	sendToBackground(message);
}

/**
 * Handle input events from the input handler
 */
function handleInputEvent(message: InputEventMessage): void {
	sendToBackground(message);
}

// ─── Recording Controls ───────────────────────────────────────────

/**
 * Start recording user interactions
 */
function startRecording(startTime?: number): void {
	if (isRecording) {
		console.warn("[HTR NControl] Recording already active");
		return;
	}

	recordingStartTime = startTime || Date.now();
	isRecording = true;

	// Start tracking clicks and inputs
	startClickTracking(recordingStartTime, handleClickEvent);
	startInputTracking(recordingStartTime, handleInputEvent);

	console.info("[HTR NControl] Recording started");
}

/**
 * Stop recording user interactions
 */
function stopRecording(): void {
	if (!isRecording) {
		console.warn("[HTR NControl] No active recording to stop");
		return;
	}

	// Flush any pending input events
	flushPendingInputs();

	// Stop tracking
	stopClickTracking();
	stopInputTracking();

	// Clean up highlighter
	removeHighlight();

	isRecording = false;
	recordingStartTime = null;

	console.info("[HTR NControl] Recording stopped");
}

// ─── Command Handling ─────────────────────────────────────────────

/**
 * Handle a remote control command from the background script.
 * Executes the command and sends the result back.
 */
async function handleCommand(
	command: Command,
	sendResponse: (response: CommandResult) => void,
): Promise<void> {
	console.info(
		`[HTR NControl] Executing command: ${command.action} (${command.id})`,
	);

	try {
		const result = await executeCommand(command);

		// Capture screenshot if requested or if the command was a screenshot request
		if (command.action === "screenshot" || command.options?.screenshot) {
			try {
				const screenshotResponse = await chrome.runtime.sendMessage({
					type: "CAPTURE_SCREENSHOT",
					tabId: await getTabId(),
				});
				if (screenshotResponse?.screenshotData) {
					result.screenshot = screenshotResponse.screenshotData;
				}
			} catch {
				// Screenshot capture might fail, that's ok
				console.warn("[HTR NControl] Screenshot capture failed");
			}
		}

		sendResponse(result);
	} catch (error) {
		console.error("[HTR NControl] Command execution failed:", error);
		sendResponse({
			id: command.id,
			success: false,
			error: error instanceof Error ? error.message : String(error),
		});
	}
}

/**
 * Get the current tab ID (helper for screenshot capture)
 */
async function getTabId(): Promise<number> {
	return new Promise((resolve) => {
		chrome.runtime.sendMessage({ type: "GET_CURRENT_TAB_ID" }, (response) => {
			resolve(response?.tabId ?? 0);
		});
	});
}

// ─── Remote Control ───────────────────────────────────────────────

/**
 * Enable remote control by connecting to the server
 */
function enableRemoteControl(_serverUrl?: string): void {
	if (remoteControlEnabled) {
		console.warn("[HTR NControl] Remote control already enabled");
		return;
	}

	remoteControlEnabled = true;
	connectRemote();
	console.info("[HTR NControl] Remote control enabled");
}

/**
 * Disable remote control
 */
function disableRemoteControl(): void {
	if (!remoteControlEnabled) {
		return;
	}

	remoteControlEnabled = false;
	disconnectRemote();
	console.info("[HTR NControl] Remote control disabled");
}

// ─── Message Handler ──────────────────────────────────────────────

/**
 * Handle messages from background script
 */
function handleMessage(
	message:
		| RecordingMessage
		| { type: "EXECUTE_COMMAND"; command: Command; tabId?: number }
		| { type: "ENABLE_REMOTE_CONTROL"; serverUrl?: string }
		| { type: "DISABLE_REMOTE_CONTROL" }
		| { type: "ENABLE_WS_REMOTE_CONTROL"; serverUrl?: string }
		| { type: "GET_CURRENT_TAB_ID" },
	_sender: chrome.runtime.MessageSender,
	sendResponse: (response?: unknown) => void,
): boolean {
	switch (message.type) {
		case "ENABLE_RECORDING": {
			const enableMsg = message as EnableRecordingMessage & {
				startTime?: number;
			};
			startRecording(enableMsg.startTime);
			sendResponse({ success: true });
			break;
		}

		case "DISABLE_RECORDING":
			stopRecording();
			sendResponse({ success: true });
			break;

		case "GET_RECORDING_STATE":
			sendResponse({
				isRecording,
				url: window.location.href,
				title: document.title,
			});
			break;

		case "HIGHLIGHT_ELEMENT": {
			const highlightMsg = message as HighlightElementMessage;
			try {
				const element = document.querySelector(highlightMsg.selector);
				if (element) {
					showHighlight(element);
					sendResponse({ success: true });
				} else {
					console.warn(
						"[HTR NControl] Element not found for selector:",
						highlightMsg.selector,
					);
					sendResponse({ success: false, error: "Element not found" });
				}
			} catch (error) {
				console.warn("[HTR NControl] Failed to highlight element:", error);
				sendResponse({ success: false, error: String(error) });
			}
			break;
		}

		case "HIDE_HIGHLIGHT":
			hideHighlight();
			sendResponse({ success: true });
			break;

		case "EXECUTE_COMMAND": {
			const cmdMsg = message as { type: "EXECUTE_COMMAND"; command: Command };
			// Handle command asynchronously
			handleCommand(cmdMsg.command, (result) => {
				sendResponse(result);
			});
			// Return true to indicate async response
			return true;
		}

		case "ENABLE_REMOTE_CONTROL": {
			const msg = message as {
				type: "ENABLE_REMOTE_CONTROL";
				serverUrl?: string;
			};
			enableRemoteControl(msg.serverUrl);
			sendResponse({ success: true });
			break;
		}

		case "ENABLE_WS_REMOTE_CONTROL": {
			const msg = message as {
				type: "ENABLE_WS_REMOTE_CONTROL";
				serverUrl?: string;
			};
			enableRemoteControl(msg.serverUrl);
			sendResponse({ success: true });
			break;
		}

		case "DISABLE_REMOTE_CONTROL":
			disableRemoteControl();
			sendResponse({ success: true });
			break;

		default:
			// Unknown message type
			break;
	}

	// Return true to indicate async response
	return true;
}

// ─── Initialize ───────────────────────────────────────────────────

// This content script can be loaded two ways: declaratively (via the
// manifest `content_scripts` entry, on page load) or programmatically
// (via `chrome.scripting.executeScript`). Both paths run this module's
// top-level code in the same isolated world.
//
// The message listener must be (re-)registered on every execution so it
// survives extension reloads: after a reload the old extension context is
// invalidated and its listeners silently die, but `__htrncontrolInitialized`
// persists on `window` (the page was not reloaded). removeListener is a
// no-op if the function isn't registered, preventing double-registration.
interface ContentScriptWindow extends Window {
	__htrncontrolInitialized?: boolean;
}
const contentWindow = window as ContentScriptWindow;

chrome.runtime.onMessage.removeListener(handleMessage);
chrome.runtime.onMessage.addListener(handleMessage);

if (!contentWindow.__htrncontrolInitialized) {
	contentWindow.__htrncontrolInitialized = true;

	// Highlight event listeners (driven by the command executor).
	window.addEventListener("htrncontrol:highlight", ((e: CustomEvent) => {
		if (e.detail?.element) {
			showHighlight(e.detail.element);
		}
	}) as EventListener);

	window.addEventListener("htrncontrol:unhighlight", () => {
		hideHighlight();
	});

	// Handle page unload - ensure any pending data is sent
	window.addEventListener("beforeunload", () => {
		if (isRecording) {
			flushPendingInputs();
		}
		// Disconnect from remote control server
		if (remoteControlEnabled) {
			disableRemoteControl();
		}
	});

	// Handle visibility change - could be used for pausing recording
	document.addEventListener("visibilitychange", () => {
		if (document.hidden && isRecording) {
			// Page became hidden, flush pending inputs
			flushPendingInputs();
		}
	});
}

// Announce readiness to the background. Runs on every execution so the
// background's `readyTabs` set is repopulated after a service-worker restart.
// Retries with backoff: at `document_start` the background may not have
// finished initialising yet, so the first send can fail silently.
function announceReady(attemptsLeft = 5): void {
	chrome.runtime
		.sendMessage({ type: "CONTENT_SCRIPT_READY", url: window.location.href })
		.catch(() => {
			if (attemptsLeft > 0) {
				setTimeout(() => announceReady(attemptsLeft - 1), 1000);
			}
		});
}
announceReady();

// Auto-enable remote control. The connection manager first checks whether
// native messaging is active (preferred) and, if not, falls back to a direct
// WebSocket connection to the remote-control server. This is what lets the
// extension connect on Firefox where native messaging may be unavailable.
connectRemote();
