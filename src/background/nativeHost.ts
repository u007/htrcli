/**
 * Native Messaging Host integration for the HTR Ncontrol extension.
 * Manages the connection to the htcli native host via Chrome Native Messaging.
 */

import type { Command, CommandResult, TabInfo } from "../types/commands";
import type { ConnectionMode } from "../types/recording";
import { AIA_API_KEY } from "../utils/aiaConfig";

const HOST_NAME = "com.howtorecorder.host";
const RECONNECT_BASE_MS = 1000;
const RECONNECT_MAX_MS = 30000;
const MAX_RECONNECT_ATTEMPTS = 20;

let nativePort: chrome.runtime.Port | null = null;
let reconnectDelay = RECONNECT_BASE_MS;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let connectionMode: "native" | "disconnected" | "unavailable" = "unavailable";
let reconnectAttempts = 0;

type ScreenshotResult = { data?: string; error?: string };
type ScreenshotCapturer = (tabId: number) => Promise<ScreenshotResult>;
let captureScreenshot: ScreenshotCapturer | null = null;

// ─── Public API ────────────────────────────────────────────────────

export function startNativeHost(): void {
	connect();
}

/**
 * Register the function used to capture a tab screenshot. Screenshots are
 * uploaded to the daemon over HTTP (not the relay) because a base64 PNG
 * routinely exceeds the 1 MB native-messaging frame limit.
 */
export function setScreenshotCapturer(fn: ScreenshotCapturer): void {
	captureScreenshot = fn;
}

export function getConnectionMode(): "native" | "disconnected" | "unavailable" {
	return connectionMode;
}

/**
 * Manually retry connecting to the native host after the max retries cap has
 * been exceeded, or any time the user wants to force a reconnection attempt.
 */
export function retryConnect(): void {
	// Reset the retry state so connect() works fresh
	reconnectAttempts = 0;
	reconnectDelay = RECONNECT_BASE_MS;
	connect();
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

		if (isPermanentError(err)) {
			markUnavailable();
			return;
		}

		// Transient: the relay exits immediately when it can't reach the daemon
		// socket, which Chrome reports as "Native host has exited" — that is NOT
		// permanent (it just means the daemon is down), and neither is a relay
		// death from the SW being killed. Retry with backoff so we recover as
		// soon as the daemon starts, instead of staying dead until the extension
		// is reloaded.
		setDisconnected();
		scheduleReconnect();
	});

	connectionMode = "native";
	reconnectDelay = RECONNECT_BASE_MS;
	reconnectAttempts = 0;
	console.log("[NativeHost] Connected");

	// Broadcast new status to all content scripts
	broadcastStatus();
	// Register any tabs that are already open so the daemon can target them
	// immediately, even if their content scripts loaded before the relay.
	syncRegisteredTabs();
}

/**
 * A native-messaging disconnect is permanent only when the host itself is
 * missing or blocked — retrying cannot fix those until the user installs the
 * host or grants access. Every other disconnect (daemon down → relay exits, or
 * the service worker was killed) is transient and must be retried with backoff.
 */
export function isPermanentError(err: string): boolean {
	return (
		err.includes("not found") ||
		err.includes("not installed") ||
		err.includes("No such native application") ||
		err.includes("Access to the specified native messaging host is forbidden")
	);
}

function setDisconnected(): void {
	connectionMode = "disconnected";
	nativePort = null;
	broadcastStatus();
}

function markUnavailable(): void {
	connectionMode = "unavailable";
	nativePort = null;
	reconnectTimer = null;
	broadcastStatus();
}

function scheduleReconnect(): void {
	reconnectAttempts++;
	if (reconnectAttempts > MAX_RECONNECT_ATTEMPTS) {
		console.warn(
			`[NativeHost] Max reconnection attempts (${MAX_RECONNECT_ATTEMPTS}) reached, giving up`,
		);
		markUnavailable();
		return;
	}

	reconnectTimer = setTimeout(() => {
		reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_MAX_MS);
		connect();
	}, reconnectDelay);
}

function syncRegisteredTabs(): void {
	if (!nativePort) return;

	chrome.tabs.query({}, (tabs) => {
		// Re-check: port may have disconnected during the async query.
		if (!nativePort) return;
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

// The background registers a listener so the native host doesn't broadcast
// on its own. This lets the background fold the native-host mode together
// with the content-script WebSocket status into a single unified mode that
// is then broadcast once to the side panel and content scripts.
let statusListener: ((mode: ConnectionMode) => void) | null = null;

export function setStatusListener(fn: (mode: ConnectionMode) => void): void {
	statusListener = fn;
}

function broadcastStatus(): void {
	statusListener?.(connectionMode);
}

interface NativeCommandMessage {
	type: "command";
	tabId: number;
	payload: Command;
}

interface NativeCaptureScreenshotMessage {
	type: "capture_screenshot";
	tabId: number;
	commandId: string;
	payload: { uploadUrl: string; token?: string };
}

interface NativeRegisterAckMessage {
	type: "ping";
}

type NativeMessage =
	| NativeCommandMessage
	| NativeCaptureScreenshotMessage
	| NativeRegisterAckMessage;

function handleNativeMessage(msg: NativeMessage): void {
	if (msg.type === "command") {
		const { tabId, payload } = msg;
		void sendCommandToTab(tabId, payload);
		return;
	}

	if (msg.type === "capture_screenshot") {
		void handleCaptureScreenshot(msg);
	}
}

/**
 * Capture a screenshot on behalf of the daemon and upload it over HTTP.
 * Always POSTs back — on failure with an error field — so the daemon's GET
 * fails fast instead of hanging until its timeout.
 */
async function handleCaptureScreenshot(
	msg: NativeCaptureScreenshotMessage,
): Promise<void> {
	const { tabId, commandId, payload } = msg;
	const { uploadUrl, token } = payload;

	let data: string | undefined;
	let errMsg = "";
	try {
		const res: ScreenshotResult = captureScreenshot
			? await captureScreenshot(tabId)
			: { error: "no screenshot capturer registered" };
		data = res.data;
		errMsg = res.error ?? (data ? "" : "screenshot capture returned no data");
		if (errMsg) {
			console.error("[NativeHost] Screenshot capture failed:", errMsg);
		}
	} catch (err) {
		errMsg = err instanceof Error ? err.message : String(err);
		console.error("[NativeHost] Screenshot capture failed:", err);
	}

	try {
		await fetch(uploadUrl, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...(token ? { Authorization: `Bearer ${token}` } : {}),
			},
			body: JSON.stringify(
				errMsg ? { commandId, error: errMsg } : { commandId, data },
			),
		});
	} catch (err) {
		// Upload failed — the daemon's GET will time out. Log loudly.
		console.error("[NativeHost] Screenshot upload failed:", err);
	}
}

// ─── Command dispatch ─────────────────────────────────────────────

function stripScreenshot(result: CommandResult): CommandResult {
	if (result.screenshot !== undefined) {
		result.screenshot = undefined;
		console.debug(
			"[NativeHost] screenshot stripped from relay result; fetch via /api/screenshot",
		);
	}
	return result;
}

function replyError(tabId: number, id: string, error: string): void {
	sendToNative({
		type: "command_result",
		tabId,
		payload: { id, success: false, error },
	});
}

async function handleFetchInPage(
	tabId: number,
	payload: Command,
): Promise<void> {
	const { id, value: url, options } = payload;
	if (!url) {
		replyError(tabId, id, "fetchInPage requires a URL (value field)");
		return;
	}
	try {
		const results = await chrome.scripting.executeScript({
			target: { tabId },
			world: "MAIN",
			// biome-ignore lint/complexity/useArrowFunction: serialized to page context
			// Synchronous XHR so no Promise is returned — avoids Chrome's known
			// issue where executeScript world:MAIN async return values are null.
			func: (
				fetchUrl: string,
				fetchMethod: string,
				extraHeaders: Record<string, string>,
				fetchBody: unknown,
			) => {
				const headers: Record<string, string> = { ...extraHeaders };
				try {
					const raw = localStorage.getItem("OAOP_LOGINDATA");
					if (raw) {
						const parsed = JSON.parse(raw) as { jwt?: string };
						if (parsed.jwt) headers["Authorization"] = `Bearer ${parsed.jwt}`;
					}
				} catch {
					// intentionally not logged: running in page context, failures are benign
				}
				// Synchronous XHR — deprecated but reliable in page MAIN world
				const xhr = new XMLHttpRequest();
				xhr.open(fetchMethod, fetchUrl, false /* synchronous */);
				// Binary-safe: treat response bytes as Latin-1 raw chars
				xhr.overrideMimeType("text/plain; charset=x-user-defined");
				Object.entries(headers).forEach(([k, v]) => {
					xhr.setRequestHeader(k, v);
				});
				xhr.withCredentials = true;
				xhr.send(fetchBody !== null ? JSON.stringify(fetchBody) : null);
				const status = xhr.status;
				const contentType = xhr.getResponseHeader("content-type") || "";
				if (status === 0 || status >= 400) {
					throw new Error(`HTTP ${status}: ${xhr.responseText.slice(0, 400)}`);
				}
				// Binary-safe base64 encode via Latin-1 charcode masking
				const text = xhr.responseText;
				let bin = "";
				for (let i = 0; i < text.length; i++) {
					bin += String.fromCharCode(text.charCodeAt(i) & 0xff);
				}
				return JSON.stringify({ status, contentType, base64: btoa(bin) });
			},
			args: [
				url,
				(options?.method as string) || "POST",
				Object.assign(
					{
						"Content-Type": "application/json",
						Accept: "application/json",
						"X-Channel-ID": "MYP_WEB",
						"X-Gateway-APIKey": AIA_API_KEY,
						"Content-Language": "en",
					},
					(options?.headers as Record<string, string>) ?? {},
				),
				// null = no body (serializable); undefined would break executeScript args
				options?.body !== undefined ? options.body : null,
			],
		});
		// Result is a JSON string (binary-safe); parse it back to an object.
		const raw = results[0]?.result as string | null | undefined;
		const data = raw ? JSON.parse(raw) : null;
		sendToNative({
			type: "command_result",
			tabId,
			payload: { id, success: true, data } as CommandResult,
		});
	} catch (err) {
		replyError(tabId, id, err instanceof Error ? err.message : String(err));
	}
}

// Evaluates an async JS expression via Chrome DevTools Protocol.
// Bypasses executeScript structured-clone issues — CDP returns values natively.
async function handleDebuggerEval(
	tabId: number,
	payload: Command,
): Promise<void> {
	const { id, value: expression } = payload;
	if (!expression) {
		replyError(tabId, id, "debuggerEval requires an expression (value field)");
		return;
	}
	try {
		await chrome.debugger.attach({ tabId }, "1.3");
		try {
			type EvalResult = {
				result: { type: string; value: unknown };
				exceptionDetails?: unknown;
			};
			const res = (await chrome.debugger.sendCommand(
				{ tabId },
				"Runtime.evaluate",
				{
					expression,
					awaitPromise: true,
					returnByValue: true,
				},
			)) as EvalResult;
			if (res.exceptionDetails) {
				throw new Error(
					`JS exception: ${JSON.stringify(res.exceptionDetails)}`,
				);
			}
			sendToNative({
				type: "command_result",
				tabId,
				payload: { id, success: true, data: res.result.value } as CommandResult,
			});
		} finally {
			await chrome.debugger.detach({ tabId });
		}
	} catch (err) {
		replyError(tabId, id, err instanceof Error ? err.message : String(err));
	}
}

// ─── Navigation with load-wait ────────────────────────────────────
// Navigation actions are handled here (not in the content script) because the
// content script is destroyed by the navigation and can only reply before the
// page unloads — making "navigate" resolve instantly without waiting for the
// load. The background persists across navigations, so it can initiate via the
// tabs API and wait for onUpdated status "complete" (Playwright waitUntil:load
// equivalent). Works on Firefox too (tabs API via polyfill, no CDP needed).

const NAV_ACTIONS = new Set(["navigate", "reload", "goBack", "goForward"]);
// Under htcli's 30s HTTP timeout and the daemon's 30s command timeout, so the
// caller gets a clean error instead of a transport timeout.
const NAV_LOAD_TIMEOUT_MS = 25000;

async function initiateNavigation(
	tabId: number,
	payload: Command,
): Promise<void> {
	switch (payload.action) {
		case "navigate":
			if (!payload.value) throw new Error("navigate requires a URL");
			await chrome.tabs.update(tabId, { url: payload.value });
			return;
		case "reload":
			await chrome.tabs.reload(tabId);
			return;
		case "goBack":
			await chrome.tabs.goBack(tabId);
			return;
		case "goForward":
			await chrome.tabs.goForward(tabId);
			return;
		default:
			throw new Error(`not a navigation action: ${payload.action}`);
	}
}

function navigateAndWaitForLoad(
	tabId: number,
	payload: Command,
): Promise<chrome.tabs.Tab> {
	return new Promise((resolve, reject) => {
		let settled = false;
		const listener = (
			updatedTabId: number,
			changeInfo: chrome.tabs.TabChangeInfo,
			tab: chrome.tabs.Tab,
		) => {
			if (settled || updatedTabId !== tabId) return;
			// Full page load, or same-document navigation (hash / history API)
			// which changes the URL without a loading→complete cycle.
			if (
				changeInfo.status === "complete" ||
				(changeInfo.url !== undefined && tab.status === "complete")
			) {
				cleanup();
				resolve(tab);
			}
		};
		const timer = setTimeout(() => {
			cleanup();
			reject(
				new Error(
					`${payload.action}: page did not finish loading within ${NAV_LOAD_TIMEOUT_MS}ms`,
				),
			);
		}, NAV_LOAD_TIMEOUT_MS);
		const cleanup = () => {
			settled = true;
			clearTimeout(timer);
			chrome.tabs.onUpdated.removeListener(listener);
		};
		// Listener must be attached before initiating so fast loads aren't missed
		chrome.tabs.onUpdated.addListener(listener);
		initiateNavigation(tabId, payload).catch((err) => {
			if (settled) return;
			cleanup();
			reject(err instanceof Error ? err : new Error(String(err)));
		});
	});
}

async function handleNavigationCommand(
	tabId: number,
	payload: Command,
): Promise<void> {
	const start = Date.now();
	try {
		const tab = await navigateAndWaitForLoad(tabId, payload);
		sendToNative({
			type: "command_result",
			tabId,
			payload: {
				id: payload.id,
				success: true,
				data: { url: tab.url, title: tab.title },
				duration: Date.now() - start,
			} as CommandResult,
		});
	} catch (err) {
		console.error(`[NativeHost] ${payload.action} failed:`, err);
		replyError(
			tabId,
			payload.id,
			err instanceof Error ? err.message : String(err),
		);
	}
}

async function sendCommandToTab(
	tabId: number,
	payload: Command,
): Promise<void> {
	if (NAV_ACTIONS.has(payload.action)) {
		await handleNavigationCommand(tabId, payload);
		return;
	}
	if (payload.action === "fetchInPage") {
		await handleFetchInPage(tabId, payload);
		return;
	}
	if (payload.action === "debuggerEval") {
		await handleDebuggerEval(tabId, payload);
		return;
	}

	const sendMsg = (): Promise<CommandResult | null> =>
		new Promise((resolve) => {
			chrome.tabs.sendMessage(
				tabId,
				{ type: "EXECUTE_COMMAND", command: payload },
				(result: CommandResult) => {
					if (chrome.runtime.lastError) {
						resolve(null);
						return;
					}
					resolve(result);
				},
			);
		});

	let result = await sendMsg();

	if (result === null) {
		// Content script not ready — try to inject it, then retry once
		try {
			const manifest = chrome.runtime.getManifest();
			const contentScriptFile =
				manifest.content_scripts?.[0]?.js?.[0] ?? "assets/chunk-DjFz53LA.js";
			await chrome.scripting.executeScript({
				target: { tabId },
				files: [contentScriptFile],
			});
			await new Promise((r) => setTimeout(r, 300));
		} catch (err) {
			console.warn("[NativeHost] content script injection failed:", err);
			replyError(tabId, payload.id, "tab not available");
			return;
		}
		result = await sendMsg();
	}

	if (result === null) {
		replyError(tabId, payload.id, "tab not available");
		return;
	}

	sendToNative({
		type: "command_result",
		tabId,
		payload: stripScreenshot(result),
	});
}

// ─── Tab registration ─────────────────────────────────────────────

export function registerTab(tabId: number, info: TabInfo): void {
	sendToNative({
		type: "register",
		tabId,
		payload: info,
	});
}
