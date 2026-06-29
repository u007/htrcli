/**
 * Background Service Worker
 * Orchestrates recording sessions, captures screenshots, manages state,
 * and forwards remote control commands to content scripts.
 */

import {
	addAnnotation as dbAddAnnotation,
	addStep as dbAddStep,
	createSession as dbCreateSession,
} from "../db/index";
import type { Command, CommandResult } from "../types/commands";
import type {
	AddAnnotationMessage,
	Annotation,
	ClickEventMessage,
	DeleteAnnotationMessage,
	InputEventMessage,
	RecordingMessage,
	RecordingSession,
	RecordingStateMessage,
	RecordingStep,
	StartRecordingMessage,
	UpdateAnnotationMessage,
} from "../types/recording";
import { getConnectionMode, registerTab, startNativeHost } from "./nativeHost";

type ChromeWithSidebarAction = typeof chrome & {
	sidebarAction?: unknown;
};

function getContentScriptInjectionFile(): string {
	const chromeApi = chrome as ChromeWithSidebarAction;
	return typeof chromeApi.sidebarAction === "undefined"
		? "src/contentScript/index.ts"
		: "content.js";
}

async function registerNativeTab(
	tabId: number,
	tab?: chrome.tabs.Tab,
): Promise<void> {
	let resolvedTab = tab;
	if (!resolvedTab) {
		try {
			resolvedTab = await chrome.tabs.get(tabId);
		} catch {
			return;
		}
	}

	if (!resolvedTab?.url || !/^https?:\/\//.test(resolvedTab.url)) return;

	registerTab(tabId, {
		id: tabId,
		url: resolvedTab.url || "",
		title: resolvedTab.title || "",
		active: resolvedTab.active || false,
		favIconUrl: resolvedTab.favIconUrl,
	});
}

/**
 * Migrate sessions from chrome.storage.local to IndexedDB (one-time migration)
 */
async function migrateFromChromeStorage(): Promise<void> {
	// Check if migration already ran
	const { migratedToIndexedDB } = await chrome.storage.local.get(
		"migratedToIndexedDB",
	);
	if (migratedToIndexedDB) return;

	console.log(
		"[How-To Recorder] Migrating sessions from chrome.storage.local to IndexedDB...",
	);

	try {
		// Get the session index
		const { sessionIndex = [] } =
			await chrome.storage.local.get("sessionIndex");

		// Migrate each session
		for (const meta of sessionIndex) {
			const result = await chrome.storage.local.get(`session_${meta.id}`);
			const session = result[`session_${meta.id}`] as
				| RecordingSession
				| undefined;

			if (!session) {
				console.warn(
					`[How-To Recorder] Session ${meta.id} not found in chrome.storage.local, skipping`,
				);
				continue;
			}

			// Save session metadata to IndexedDB
			await dbCreateSession(session);

			// Save steps
			for (const step of session.steps) {
				await dbAddStep(session.id, step);
			}

			// Save annotations
			for (const annotation of session.annotations) {
				await dbAddAnnotation(session.id, annotation);
			}

			console.log(
				`[How-To Recorder] Migrated session ${meta.id} (${session.steps.length} steps, ${session.annotations.length} annotations)`,
			);
		}

		// Clean up old storage keys
		const keysToRemove = sessionIndex.map(
			(s: { id: string }) => `session_${s.id}`,
		);
		if (keysToRemove.length > 0) {
			await chrome.storage.local.remove(keysToRemove);
		}

		// Mark migration as complete
		await chrome.storage.local.set({ migratedToIndexedDB: true });
		console.log(
			`[How-To Recorder] Migration complete: ${sessionIndex.length} sessions migrated`,
		);
	} catch (error) {
		console.error("[How-To Recorder] Migration failed:", error);
		// Don't set the flag so it retries on next startup
	}
}

console.log("[How-To Recorder] Background service worker started");

// Run migration on startup
migrateFromChromeStorage();

// Current recording session
let currentSession: RecordingSession | null = null;

// Track which tabs have content scripts ready
const readyTabs = new Set<number>();

// Generate unique IDs
function generateUniqueId(prefix: string = ""): string {
	return `${prefix}${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
}

/**
 * Capture screenshot of a tab
 */
async function captureScreenshot(tabId: number): Promise<string | undefined> {
	try {
		// Get the tab to find its windowId
		const tab = await chrome.tabs.get(tabId);
		if (!tab.windowId) {
			console.warn("[How-To Recorder] Tab has no windowId");
			return undefined;
		}

		const dataUrl = await chrome.tabs.captureVisibleTab(tab.windowId, {
			format: "png",
			quality: 90,
		});
		return dataUrl;
	} catch (error) {
		console.warn("[How-To Recorder] Failed to capture screenshot:", error);
		return undefined;
	}
}

/**
 * Highlight an element in a tab before screenshot
 */
async function highlightElement(
	tabId: number,
	selector: string,
): Promise<boolean> {
	try {
		await chrome.tabs.sendMessage(tabId, {
			type: "HIGHLIGHT_ELEMENT",
			selector,
		});
		// Wait for highlight to render
		await new Promise((resolve) => setTimeout(resolve, 100));
		return true;
	} catch (error) {
		console.warn("[How-To Recorder] Failed to highlight element:", error);
		return false;
	}
}

/**
 * Hide the highlight overlay in a tab
 */
async function hideHighlightInTab(tabId: number): Promise<void> {
	try {
		await chrome.tabs.sendMessage(tabId, { type: "HIDE_HIGHLIGHT" });
	} catch (error) {
		console.warn("[How-To Recorder] Failed to hide highlight:", error);
	}
}

/**
 * Capture screenshot with optional element highlighting
 */
async function captureScreenshotWithHighlight(
	tabId: number,
	selector?: string,
): Promise<string | undefined> {
	// If we have a selector, highlight the element first
	if (selector) {
		await highlightElement(tabId, selector);
	}

	// Capture the screenshot
	const screenshotData = await captureScreenshot(tabId);

	// Hide the highlight after capturing
	if (selector) {
		await hideHighlightInTab(tabId);
	}

	return screenshotData;
}

/**
 * Enable recording in a specific tab
 */
async function enableRecordingInTab(
	tabId: number,
	startTime: number,
): Promise<void> {
	try {
		await chrome.tabs.sendMessage(tabId, {
			type: "ENABLE_RECORDING",
			startTime,
		});
		console.log(`[How-To Recorder] Recording enabled in tab ${tabId}`);
	} catch (error) {
		// Tab might not have content script loaded, try to inject it
		console.warn(
			`[How-To Recorder] Could not enable recording in tab ${tabId}:`,
			error,
		);
		try {
			await chrome.scripting.executeScript({
				target: { tabId },
				files: [getContentScriptInjectionFile()],
			});
			// Try again after injection
			await chrome.tabs.sendMessage(tabId, {
				type: "ENABLE_RECORDING",
				startTime,
			});
		} catch (injectError) {
			console.warn(
				`[How-To Recorder] Could not inject content script in tab ${tabId}:`,
				injectError,
			);
		}
	}
}

/**
 * Disable recording in a specific tab
 */
async function disableRecordingInTab(tabId: number): Promise<void> {
	try {
		await chrome.tabs.sendMessage(tabId, { type: "DISABLE_RECORDING" });
		console.log(`[How-To Recorder] Recording disabled in tab ${tabId}`);
	} catch (error) {
		console.warn(
			`[How-To Recorder] Could not disable recording in tab ${tabId}:`,
			error,
		);
	}
}

/**
 * Start a new recording session
 */
async function startRecording(
	title: string,
	hasAudio: boolean,
): Promise<RecordingSession> {
	// Get the current active tab
	const [activeTab] = await chrome.tabs.query({
		active: true,
		currentWindow: true,
	});

	if (!activeTab?.id) {
		throw new Error("No active tab found");
	}

	const startTime = Date.now();

	// Create new session
	currentSession = {
		id: generateUniqueId("session_"),
		title,
		startTime,
		isRecording: true,
		hasAudio,
		steps: [],
		annotations: [],
		trackedTabIds: [activeTab.id],
	};

	// Enable recording in the active tab
	await enableRecordingInTab(activeTab.id, startTime);

	// Capture initial navigation step
	const initialStep: RecordingStep = {
		id: generateUniqueId("step_"),
		timestamp: 0,
		type: "navigation",
		tabId: activeTab.id,
		tabTitle: activeTab.title || "Untitled",
		url: activeTab.url || "",
		screenshotData: await captureScreenshot(activeTab.id),
	};

	currentSession.steps.push(initialStep);

	// Notify side panel about the new step
	broadcastToSidePanel({
		type: "NEW_STEP",
		step: initialStep,
	});

	// Save session metadata to storage
	await saveSessionMetadata();

	console.log("[How-To Recorder] Recording started:", currentSession.id);

	return currentSession;
}

/**
 * Stop the current recording session
 */
async function stopRecording(): Promise<RecordingSession | null> {
	if (!currentSession) {
		console.warn("[How-To Recorder] No active recording to stop");
		return null;
	}

	currentSession.isRecording = false;
	currentSession.endTime = Date.now();

	// Disable recording in all tracked tabs
	for (const tabId of currentSession.trackedTabIds) {
		await disableRecordingInTab(tabId);
	}

	// Save final session data
	try {
		await saveSessionMetadata();
	} catch (error) {
		console.error("[How-To Recorder] Failed to save session metadata:", error);
	}

	const finishedSession = currentSession;
	console.log("[How-To Recorder] Recording stopped:", finishedSession.id);

	return finishedSession;
}

/**
 * Add a step to the current recording
 */
async function addStep(
	type: "click" | "input" | "navigation",
	data: Partial<RecordingStep>,
	tabId: number,
): Promise<RecordingStep | null> {
	if (!currentSession || !currentSession.isRecording) {
		return null;
	}

	// Get tab info
	let tabTitle = "Unknown";
	let tabUrl = "";
	try {
		const tab = await chrome.tabs.get(tabId);
		tabTitle = tab.title || "Untitled";
		tabUrl = tab.url || data.url || "";
	} catch {
		tabUrl = data.url || "";
	}

	// Capture screenshot with element highlight for click/input events
	const selector = data.element?.selector;
	const screenshotData = await captureScreenshotWithHighlight(tabId, selector);

	const step: RecordingStep = {
		id: generateUniqueId("step_"),
		timestamp: data.timestamp || Date.now() - currentSession.startTime,
		type,
		tabId,
		tabTitle,
		url: tabUrl,
		screenshotData,
		element: data.element,
		inputValue: data.inputValue,
		isSensitive: data.isSensitive,
	};

	currentSession.steps.push(step);

	// Notify side panel
	broadcastToSidePanel({
		type: "NEW_STEP",
		step,
	});

	// Add tab to tracked tabs if not already
	if (!currentSession.trackedTabIds.includes(tabId)) {
		currentSession.trackedTabIds.push(tabId);
	}

	return step;
}

/**
 * Add an annotation to the current recording
 */
function addAnnotation(text: string, timestamp: number): Annotation | null {
	if (!currentSession) {
		return null;
	}

	const annotation: Annotation = {
		id: generateUniqueId("ann_"),
		timestamp,
		text,
	};

	currentSession.annotations.push(annotation);

	return annotation;
}

/**
 * Update an annotation
 */
function updateAnnotation(
	annotationId: string,
	text: string,
): Annotation | null {
	if (!currentSession) {
		return null;
	}

	const annotation = currentSession.annotations.find(
		(a) => a.id === annotationId,
	);
	if (annotation) {
		annotation.text = text;
		return annotation;
	}

	return null;
}

/**
 * Delete an annotation
 */
function deleteAnnotation(annotationId: string): boolean {
	if (!currentSession) {
		return false;
	}

	const index = currentSession.annotations.findIndex(
		(a) => a.id === annotationId,
	);
	if (index !== -1) {
		currentSession.annotations.splice(index, 1);
		return true;
	}

	return false;
}

/**
 * Save session data to IndexedDB (screenshots too large for chrome.storage.local)
 */
async function saveSessionMetadata(): Promise<void> {
	if (!currentSession) return;

	// Save session metadata to IndexedDB
	await dbCreateSession(currentSession);

	// Save each step to IndexedDB (includes screenshotData)
	for (const step of currentSession.steps) {
		await dbAddStep(currentSession.id, step);
	}

	// Save annotations to IndexedDB
	for (const annotation of currentSession.annotations) {
		await dbAddAnnotation(currentSession.id, annotation);
	}

	// Keep a lightweight sessionIndex in chrome.storage.local for quick listing
	const metadata = {
		id: currentSession.id,
		title: currentSession.title,
		startTime: currentSession.startTime,
		endTime: currentSession.endTime,
		hasAudio: currentSession.hasAudio,
		stepCount: currentSession.steps.length,
		annotationCount: currentSession.annotations.length,
	};

	const { sessionIndex = [] } = await chrome.storage.local.get("sessionIndex");
	const existingIndex = sessionIndex.findIndex(
		(s: { id: string }) => s.id === metadata.id,
	);
	if (existingIndex >= 0) {
		sessionIndex[existingIndex] = metadata;
	} else {
		sessionIndex.unshift(metadata);
	}
	await chrome.storage.local.set({ sessionIndex });
}

/**
 * Broadcast message to side panel
 */
function broadcastToSidePanel(message: RecordingMessage): void {
	chrome.runtime.sendMessage(message).catch(() => {
		// Side panel might not be open
	});
}

// ─── Remote Control: Command Forwarding ────────────────────────────

/**
 * Forward a command to a content script in the specified tab.
 * If no tabId is provided, uses the active tab.
 */
async function forwardCommand(
	command: Command,
	tabId?: number,
): Promise<CommandResult> {
	// If no tabId, get the active tab
	let targetTabId = tabId;
	if (!targetTabId) {
		const [activeTab] = await chrome.tabs.query({
			active: true,
			currentWindow: true,
		});
		if (!activeTab?.id) {
			return {
				id: command.id,
				success: false,
				error: "No active tab found",
			};
		}
		targetTabId = activeTab.id;
	}

	// Ensure content script is injected
	if (!readyTabs.has(targetTabId)) {
		try {
			await chrome.scripting.executeScript({
				target: { tabId: targetTabId },
				files: [getContentScriptInjectionFile()],
			});
			// Wait a moment for script to initialize
			await new Promise((resolve) => setTimeout(resolve, 200));
		} catch (error) {
			return {
				id: command.id,
				success: false,
				error: `Failed to inject content script: ${error}`,
			};
		}
	}

	// Send command to content script
	try {
		const result = await chrome.tabs.sendMessage(targetTabId, {
			type: "EXECUTE_COMMAND",
			command,
		});
		return result as CommandResult;
	} catch (error) {
		return {
			id: command.id,
			success: false,
			error: `Failed to send command to content script: ${error}`,
		};
	}
}

/**
 * Get info about all tabs (for remote control)
 */
async function getTabsInfo(): Promise<
	Array<{ id: number; url: string; title: string; active: boolean }>
> {
	const tabs = await chrome.tabs.query({});
	return tabs.map((tab) => ({
		id: tab.id || 0,
		url: tab.url || "",
		title: tab.title || "",
		active: tab.active || false,
	}));
}

async function getReadyTabsInfo(): Promise<
	Array<{
		id: number;
		url: string;
		title: string;
		active: boolean;
		favIconUrl?: string;
	}>
> {
	const tabs = await Promise.all(
		[...readyTabs].map(async (tabId) => {
			try {
				const tab = await chrome.tabs.get(tabId);
				if (!tab.url || !/^https?:\/\//.test(tab.url)) return null;

				return {
					id: tab.id ?? tabId,
					url: tab.url || "",
					title: tab.title || "",
					active: tab.active || false,
					favIconUrl: tab.favIconUrl,
				};
			} catch {
				return null;
			}
		}),
	);

	return tabs.filter(
		(tab): tab is NonNullable<(typeof tabs)[number]> => tab !== null,
	);
}

/**
 * Ensure the content script is running in every open http(s) tab.
 *
 * The declarative `content_scripts` manifest entry only runs on page
 * load/navigation, so tabs that were already open when the extension
 * was installed/enabled (or that were open before the user granted host
 * access on Firefox) never get a content script and therefore never
 * register as "ready". This walks the open tabs and programmatically
 * injects the content script into any that aren't ready yet, using the
 * file list declared in the manifest so the same code works for both
 * the Chrome (crxjs, hashed filenames) and Firefox (`content.js`)
 * builds.
 *
 * Injection requires host access for each tab; on Firefox (where
 * `<all_urls>` is opt-in) the calls below simply fail until the user
 * grants access, which is why the side panel exposes a "Grant access"
 * action that requests it before calling this.
 */
async function ensureContentScriptsInjected(): Promise<void> {
	const manifest = chrome.runtime.getManifest();
	const files = manifest.content_scripts?.[0]?.js ?? [];
	if (files.length === 0) return;

	let tabs: chrome.tabs.Tab[];
	try {
		tabs = await chrome.tabs.query({ url: ["http://*/*", "https://*/*"] });
	} catch {
		return;
	}

	await Promise.all(
		tabs.map(async (tab) => {
			if (typeof tab.id !== "number") return;
			if (readyTabs.has(tab.id)) return;
			try {
				await chrome.scripting.executeScript({
					target: { tabId: tab.id },
					files,
				});
			} catch {
				// No host access for this tab (common on Firefox until
				// the user grants it), or a restricted page — skip it.
			}
		}),
	);
}

/**
 * Capture screenshot of a specific tab (for remote control)
 */
async function captureTabScreenshot(
	tabId: number,
): Promise<string | undefined> {
	return captureScreenshot(tabId);
}

// ─── Message Handler ──────────────────────────────────────────────

/**
 * Handle messages from content scripts and side panel
 */
chrome.runtime.onMessage.addListener(
	(
		message:
			| RecordingMessage
			| { type: "EXECUTE_COMMAND"; command: Command; tabId?: number }
			| { type: "GET_CURRENT_TAB_ID" }
			| { type: "GET_TABS_INFO" }
			| { type: "GET_TAB_INFO"; tabId?: number }
			| { type: "SWITCH_TAB"; tabId: number }
			| { type: "CAPTURE_SCREENSHOT"; tabId: number }
			| { type: "GET_RECORDING_STATE" }
			| { type: "GET_CONNECTION_STATUS" }
			| { type: "GET_TAB_ID" }
			| {
					type: "FETCH_URL";
					url: string;
					method?: string;
					headers?: Record<string, string>;
					body?: unknown;
			  }
			| { type: "GET_READY_TABS" }
			| { type: "SYNC_READY_TABS" }
			| { type: "PRINT_TO_PDF"; tabId: number }
			| { type: "OPEN_TAB"; url: string; sessionData?: string }
			| { type: "CDP_NAVIGATE"; tabId: number; url: string }
			| { type: "CLOSE_TAB"; tabId: number },
		sender,
		sendResponse,
	) => {
		const handleAsync = async () => {
			switch (message.type) {
				case "START_RECORDING": {
					const msg = message as StartRecordingMessage;
					try {
						const session = await startRecording(msg.title, msg.hasAudio);
						sendResponse({ success: true, session });
					} catch (error) {
						sendResponse({ success: false, error: String(error) });
					}
					break;
				}

				case "STOP_RECORDING": {
					try {
						const session = await stopRecording();
						sendResponse({ success: true, session });
					} catch (error) {
						console.error("[How-To Recorder] Failed to stop recording:", error);
						sendResponse({ success: false, error: String(error) });
					}
					break;
				}

				case "CLICK_EVENT": {
					const msg = message as ClickEventMessage;
					if (sender.tab?.id) {
						const step = await addStep(
							"click",
							{
								timestamp: msg.timestamp,
								element: msg.element,
								url: msg.url,
							},
							sender.tab.id,
						);
						sendResponse({ success: true, step });
					}
					break;
				}

				case "INPUT_EVENT": {
					const msg = message as InputEventMessage;
					console.log(
						"[How-To Recorder] Received INPUT_EVENT:",
						msg.element?.selector,
						"value:",
						msg.isSensitive ? "(sensitive)" : msg.value,
					);
					if (sender.tab?.id) {
						const step = await addStep(
							"input",
							{
								timestamp: msg.timestamp,
								element: msg.element,
								inputValue: msg.value,
								isSensitive: msg.isSensitive,
								url: msg.url,
							},
							sender.tab.id,
						);
						sendResponse({ success: true, step });
					} else {
						console.warn(
							"[How-To Recorder] INPUT_EVENT received but no sender.tab.id",
						);
						sendResponse({ success: false, error: "No tab id" });
					}
					break;
				}

				case "ADD_ANNOTATION": {
					const msg = message as AddAnnotationMessage;
					const annotation = addAnnotation(msg.text, msg.timestamp);
					sendResponse({ success: true, annotation });
					break;
				}

				case "UPDATE_ANNOTATION": {
					const msg = message as UpdateAnnotationMessage;
					const annotation = updateAnnotation(msg.annotationId, msg.text);
					sendResponse({ success: !!annotation, annotation });
					break;
				}

				case "DELETE_ANNOTATION": {
					const msg = message as DeleteAnnotationMessage;
					const success = deleteAnnotation(msg.annotationId);
					sendResponse({ success });
					break;
				}

				case "GET_RECORDING_STATE": {
					const response: RecordingStateMessage = {
						type: "RECORDING_STATE",
						isRecording: currentSession?.isRecording || false,
						session: currentSession || undefined,
					};
					sendResponse(response);
					break;
				}

				case "CONTENT_SCRIPT_READY": {
					if (sender.tab?.id) {
						readyTabs.add(sender.tab.id);
						void registerNativeTab(sender.tab.id, sender.tab);
						// If we're recording, enable in this tab
						if (currentSession?.isRecording) {
							await enableRecordingInTab(
								sender.tab.id,
								currentSession.startTime,
							);
							if (!currentSession.trackedTabIds.includes(sender.tab.id)) {
								currentSession.trackedTabIds.push(sender.tab.id);
							}
						}
					}
					sendResponse({ success: true });
					break;
				}

				// ─── Remote Control Commands ────────────────────────────

				case "EXECUTE_COMMAND": {
					const msg = message as {
						type: "EXECUTE_COMMAND";
						command: Command;
						tabId?: number;
					};
					const result = await forwardCommand(msg.command, msg.tabId);
					sendResponse(result);
					break;
				}

				case "GET_TABS_INFO": {
					const tabs = await getTabsInfo();
					sendResponse({ success: true, tabs });
					break;
				}

				case "GET_READY_TABS": {
					const tabs = await getReadyTabsInfo();
					sendResponse({ success: true, tabs });
					break;
				}

				case "SYNC_READY_TABS": {
					// Inject the content script into any open http(s) tabs
					// that aren't connected yet (used after the user grants
					// host access), then return the refreshed list. The
					// injected scripts announce readiness asynchronously, so
					// give them a brief moment before reporting back.
					await ensureContentScriptsInjected();
					await new Promise((resolve) => setTimeout(resolve, 300));
					const tabs = await getReadyTabsInfo();
					sendResponse({ success: true, tabs });
					break;
				}

				case "GET_CURRENT_TAB_ID": {
					// Return the tab ID of the sender (content script tab)
					sendResponse({ tabId: sender.tab?.id ?? null });
					break;
				}

				case "GET_TAB_INFO": {
					const msg = message as { type: "GET_TAB_INFO"; tabId?: number };
					const targetTabId = msg.tabId ?? sender.tab?.id;
					if (!targetTabId) {
						sendResponse({
							success: false,
							error: "No tab ID specified and sender has no tab",
						});
						break;
					}
					try {
						const tab = await chrome.tabs.get(targetTabId);
						sendResponse({
							success: true,
							tab: {
								id: tab.id || 0,
								url: tab.url || "",
								title: tab.title || "",
								active: tab.active || false,
							},
						});
					} catch (error) {
						sendResponse({
							success: false,
							error: `Tab ${targetTabId} not found: ${error instanceof Error ? error.message : String(error)}`,
						});
					}
					break;
				}

				case "GET_CONNECTION_STATUS":
					sendResponse({
						type: "CONNECTION_STATUS",
						mode: getConnectionMode(),
					});
					return true;

				case "GET_TAB_ID":
					sendResponse({ tabId: sender.tab?.id ?? 0 });
					return true;

				case "SWITCH_TAB": {
					const msg = message as { type: "SWITCH_TAB"; tabId: number };
					try {
						await chrome.tabs.update(msg.tabId, { active: true });
						sendResponse({ success: true });
					} catch (error) {
						sendResponse({
							success: false,
							error: `Failed to switch to tab ${msg.tabId}: ${error instanceof Error ? error.message : String(error)}`,
						});
					}
					break;
				}

				case "CAPTURE_SCREENSHOT": {
					const msg = message as { type: "CAPTURE_SCREENSHOT"; tabId: number };
					const screenshot = await captureTabScreenshot(msg.tabId);
					sendResponse({ screenshotData: screenshot });
					break;
				}

				case "PRINT_TO_PDF": {
					const msg = message as { type: "PRINT_TO_PDF"; tabId: number };
					const target = { tabId: msg.tabId };
					try {
						await chrome.debugger.attach(target, "1.3");
						try {
							await chrome.debugger.sendCommand(target, "Page.enable");
							const result = (await chrome.debugger.sendCommand(
								target,
								"Page.printToPDF",
								{
									printBackground: true,
									preferCSSPageSize: true,
								},
							)) as { data: string };
							sendResponse({ ok: true, data: result.data });
						} finally {
							await chrome.debugger.detach(target);
						}
					} catch (error) {
						console.error("[How-To Recorder] PRINT_TO_PDF error:", error);
						sendResponse({
							ok: false,
							error: error instanceof Error ? error.message : String(error),
						});
					}
					break;
				}

				case "FETCH_URL": {
					const msg = message as {
						type: "FETCH_URL";
						url: string;
						method?: string;
						headers?: Record<string, string>;
						body?: unknown;
					};
					try {
						const resp = await fetch(msg.url, {
							method: msg.method || "GET",
							credentials: "include",
							headers: msg.headers || { "Content-Type": "application/json" },
							body:
								msg.body !== undefined ? JSON.stringify(msg.body) : undefined,
						});
						const text = await resp.text();
						let data: unknown;
						try {
							data = JSON.parse(text);
						} catch {
							data = text;
						}
						sendResponse({ ok: resp.ok, status: resp.status, data });
					} catch (error) {
						console.error("[How-To Recorder] FETCH_URL error:", error);
						sendResponse({
							ok: false,
							error: error instanceof Error ? error.message : String(error),
						});
					}
					break;
				}

				case "OPEN_TAB": {
					const msg = message as {
						type: "OPEN_TAB";
						url: string;
						sessionData?: string;
					};
					try {
						const tab = await chrome.tabs.create({
							url: msg.url,
							active: false,
						});
						if (tab.id == null) {
							throw new Error("Opened tab has no id");
						}
						const tabId = tab.id;
						// Wait for initial load
						await new Promise<void>((resolve) => {
							const onUpdated = (
								tid: number,
								info: chrome.tabs.TabChangeInfo,
							) => {
								if (tid === tabId && info.status === "complete") {
									chrome.tabs.onUpdated.removeListener(onUpdated);
									resolve();
								}
							};
							chrome.tabs.onUpdated.addListener(onUpdated);
							// Fallback: 15s
							setTimeout(resolve, 15000);
						});
						if (msg.sessionData) {
							// Inject sessionStorage into the page (main world)
							await chrome.scripting.executeScript({
								target: { tabId },
								world: "MAIN",
								func: (data: string) => {
									sessionStorage.setItem("eReceiptData", data);
								},
								args: [msg.sessionData],
							});
							// Navigate to the preview URL (re-run in case tab redirected)
							await chrome.tabs.update(tabId, { url: msg.url });
							await new Promise<void>((resolve) => {
								const onUpdated = (
									tid: number,
									info: chrome.tabs.TabChangeInfo,
								) => {
									if (tid === tabId && info.status === "complete") {
										chrome.tabs.onUpdated.removeListener(onUpdated);
										resolve();
									}
								};
								chrome.tabs.onUpdated.addListener(onUpdated);
								setTimeout(resolve, 15000);
							});
						}
						sendResponse({ ok: true, tabId });
					} catch (error) {
						console.error("[How-To Recorder] OPEN_TAB error:", error);
						sendResponse({
							ok: false,
							error: error instanceof Error ? error.message : String(error),
						});
					}
					break;
				}

				case "CDP_NAVIGATE": {
					const msg = message as {
						type: "CDP_NAVIGATE";
						tabId: number;
						url: string;
					};
					const target = { tabId: msg.tabId };
					try {
						await chrome.debugger.attach(target, "1.3");
						try {
							await chrome.debugger.sendCommand(target, "Page.navigate", {
								url: msg.url,
							});
						} finally {
							await chrome.debugger.detach(target);
						}
						sendResponse({ ok: true });
					} catch (error) {
						console.error("[How-To Recorder] CDP_NAVIGATE error:", error);
						sendResponse({
							ok: false,
							error: error instanceof Error ? error.message : String(error),
						});
					}
					break;
				}

				case "CLOSE_TAB": {
					const msg = message as { type: "CLOSE_TAB"; tabId: number };
					try {
						await chrome.tabs.remove(msg.tabId);
						sendResponse({ ok: true });
					} catch (error) {
						console.error("[How-To Recorder] CLOSE_TAB error:", error);
						sendResponse({
							ok: false,
							error: error instanceof Error ? error.message : String(error),
						});
					}
					break;
				}

				default:
					sendResponse({ success: false, error: "Unknown message type" });
			}
		};

		handleAsync().catch((error) => {
			console.error(
				"[How-To Recorder] Unhandled error in message handler:",
				error,
			);
		});
		return true; // Indicates async response
	},
);

// ─── Tab Tracking ──────────────────────────────────────────────────

// Track tab navigation for recording new pages
chrome.tabs.onUpdated.addListener(async (tabId, changeInfo, tab) => {
	if (changeInfo.status === "complete") {
		void registerNativeTab(tabId, tab);
	}

	if (!currentSession?.isRecording) return;
	if (!currentSession.trackedTabIds.includes(tabId)) return;

	// Only track completed navigations
	if (changeInfo.status === "complete" && tab.url) {
		// Add navigation step
		const _step = await addStep(
			"navigation",
			{
				timestamp: Date.now() - currentSession.startTime,
				url: tab.url,
			},
			tabId,
		);

		console.log("[How-To Recorder] Navigation detected:", tab.url);
	}
});

// Track new tabs created from tracked tabs
chrome.tabs.onCreated.addListener(async (tab) => {
	if (!currentSession?.isRecording) return;
	if (!tab.id) return;

	// Check if this tab was opened from a tracked tab
	if (
		tab.openerTabId &&
		currentSession.trackedTabIds.includes(tab.openerTabId)
	) {
		// Add this tab to tracked tabs
		currentSession.trackedTabIds.push(tab.id);

		// Enable recording when the tab is ready
		chrome.tabs.onUpdated.addListener(function listener(tabId, changeInfo) {
			if (tabId === tab.id && changeInfo.status === "complete") {
				chrome.tabs.onUpdated.removeListener(listener);
				if (currentSession?.isRecording && tab.id) {
					enableRecordingInTab(tab.id, currentSession.startTime);
				}
			}
		});

		console.log("[How-To Recorder] New tab opened from tracked tab:", tab.id);
	}
});

// Handle tab closure
chrome.tabs.onRemoved.addListener((tabId) => {
	readyTabs.delete(tabId);

	if (currentSession?.isRecording) {
		const index = currentSession.trackedTabIds.indexOf(tabId);
		if (index !== -1) {
			currentSession.trackedTabIds.splice(index, 1);
			console.log("[How-To Recorder] Tracked tab closed:", tabId);

			// If all tracked tabs are closed, stop recording
			if (currentSession.trackedTabIds.length === 0) {
				console.log(
					"[How-To Recorder] All tracked tabs closed, stopping recording",
				);
				stopRecording()
					.then((session) => {
						if (session) {
							broadcastToSidePanel({
								type: "RECORDING_STOPPED",
								sessionId: session.id,
							});
						}
					})
					.catch((error) => {
						console.error(
							"[How-To Recorder] Failed to auto-stop recording:",
							error,
						);
					});
			}
		}
	}
});

// Open side panel when extension icon is clicked
chrome.action.onClicked.addListener(async (tab) => {
	if (tab.windowId) {
		// Use setOptions to open the side panel for this tab
		await chrome.sidePanel.setOptions({
			tabId: tab.id,
			path: "sidepanel.html",
			enabled: true,
		});
	}
});

// Set up side panel behavior
chrome.sidePanel
	.setPanelBehavior({ openPanelOnActionClick: true })
	.catch(() => {
		// API might not be available in all contexts
	});

// Start native host connection
startNativeHost();

// Connect already-open tabs on install/enable and on browser startup.
// Declarative content scripts only run on navigation, so without this a
// freshly installed/enabled extension shows zero connected tabs until
// every tab is manually reloaded. Injection is best-effort and silently
// skips tabs we lack host access for (e.g. before the user opts in on
// Firefox).
chrome.runtime.onInstalled.addListener(() => {
	void ensureContentScriptsInjected();
});
chrome.runtime.onStartup.addListener(() => {
	void ensureContentScriptsInjected();
});
