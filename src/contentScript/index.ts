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
import { hideHighlight, removeHighlight, showHighlight } from "./highlighter";
import {
	flushPendingInputs,
	startInputTracking,
	stopInputTracking,
} from "./inputHandler";
import { connect as connectRemote, disconnect as disconnectRemote, isConnected as remoteIsConnected } from "./connectionManager";

console.info("[How-To Recorder] Content script loaded");

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
		console.warn("[How-To Recorder] Failed to send message:", error);
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
		console.warn("[How-To Recorder] Recording already active");
		return;
	}

	recordingStartTime = startTime || Date.now();
	isRecording = true;

	// Start tracking clicks and inputs
	startClickTracking(recordingStartTime, handleClickEvent);
	startInputTracking(recordingStartTime, handleInputEvent);

	console.info("[How-To Recorder] Recording started");
}

/**
 * Stop recording user interactions
 */
function stopRecording(): void {
	if (!isRecording) {
		console.warn("[How-To Recorder] No active recording to stop");
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

	console.info("[How-To Recorder] Recording stopped");
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
		`[How-To Recorder] Executing command: ${command.action} (${command.id})`,
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
				console.warn("[How-To Recorder] Screenshot capture failed");
			}
		}

		sendResponse(result);
	} catch (error) {
		console.error("[How-To Recorder] Command execution failed:", error);
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
		console.warn("[How-To Recorder] Remote control already enabled");
		return;
	}

	remoteControlEnabled = true;
	connectRemote();
	console.info("[How-To Recorder] Remote control enabled");
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
	console.info("[How-To Recorder] Remote control disabled");
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
						"[How-To Recorder] Element not found for selector:",
						highlightMsg.selector,
					);
					sendResponse({ success: false, error: "Element not found" });
				}
			} catch (error) {
				console.warn("[How-To Recorder] Failed to highlight element:", error);
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

// ─── Highlight Event Listeners (for command executor) ──────────────

window.addEventListener("how-to-recorder:highlight", ((e: CustomEvent) => {
	if (e.detail?.element) {
		showHighlight(e.detail.element);
	}
}) as EventListener);

window.addEventListener("how-to-recorder:unhighlight", () => {
	hideHighlight();
});

// ─── Initialize ───────────────────────────────────────────────────

// Listen for messages from background script
chrome.runtime.onMessage.addListener(handleMessage);

// Notify background script that content script is ready
chrome.runtime
	.sendMessage({ type: "CONTENT_SCRIPT_READY", url: window.location.href })
	.catch(() => {
		// Ignore errors if background script isn't listening yet
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
