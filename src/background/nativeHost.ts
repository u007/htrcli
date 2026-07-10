/**
 * Native Messaging Host integration for the HTR NControl extension.
 * Manages the connection to the htcli native host via Chrome Native Messaging.
 */

import type { Command, CommandResult, TabInfo } from "../types/commands";
import type { ConnectionMode } from "../types/recording";
import { AIA_API_KEY } from "../utils/aiaConfig";
import { cdpEvaluate } from "./cdpEval";
import {
	CDP_INPUT_ACTIONS,
	ContentScriptNotReadyError,
	dispatchCdpInput,
} from "./cdpInput";

const HOST_NAME = "com.htrcontrol.host";
const RECONNECT_BASE_MS = 1000;
const RECONNECT_MAX_MS = 30000;
const MAX_RECONNECT_ATTEMPTS = 20;

let nativePort: chrome.runtime.Port | null = null;
let reconnectDelay = RECONNECT_BASE_MS;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let connectionMode: "native" | "disconnected" | "unavailable" = "unavailable";
let reconnectAttempts = 0;
// True once the daemon has sent at least one message over the current port.
// connectNative alone is not proof of a working chain — see confirmConnected.
let portConfirmed = false;

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

// Provides the background's readyTabs view (tabs with a live content script)
// for the native `getReadyTabs` action. Registered by background/index.ts
// (same pattern as setScreenshotCapturer) to avoid a circular import.
type ReadyTabsProvider = () => Promise<TabInfo[]>;
let readyTabsProvider: ReadyTabsProvider | null = null;

export function setReadyTabsProvider(fn: ReadyTabsProvider): void {
	readyTabsProvider = fn;
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

	// Never hold two ports at once. A leaked old port causes an eternal
	// reap/reconnect cycle: its pings get answered on the NEW port (sendToNative
	// uses nativePort), so the daemon reaps the old one as stale; its
	// onDisconnect then used to clobber the healthy port's state and spawn yet
	// another port, orphaning the previous one — repeating forever.
	if (nativePort) {
		const old = nativePort;
		nativePort = null;
		try {
			old.disconnect();
		} catch (err) {
			// intentionally not logged as error: disconnecting an already-dead
			// port throws in some browsers and is harmless here.
			console.warn("[NativeHost] old port disconnect:", err);
		}
	}

	let port: chrome.runtime.Port;
	try {
		port = chrome.runtime.connectNative(HOST_NAME);
	} catch (err) {
		console.warn("[NativeHost] connectNative failed:", err);
		markUnavailable();
		return;
	}
	nativePort = port;

	port.onMessage.addListener((msg) => {
		// Ignore traffic from a superseded port so it can't confirm the
		// connection or trigger replies on the current port.
		if (nativePort !== port) return;
		handleNativeMessage(msg as NativeMessage);
	});
	port.onDisconnect.addListener(() => {
		// A stale port's disconnect must not clobber the current port's state.
		if (nativePort !== port) return;

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

	// connectNative succeeds even when the daemon is down (the relay spawns,
	// fails to dial the daemon socket, and only then disconnects the port), so
	// do NOT report "native" yet. The daemon greets every new relay with a
	// ping; the first daemon message confirms the chain end-to-end (see
	// confirmConnected). Until then we stay "disconnected" and keep the
	// backoff counters untouched so a dead daemon doesn't reset them.
	portConfirmed = false;
	// Reflect the awaiting-greeting state. This keeps getConnectionMode()
	// honest (we have a port but no confirmed daemon) and matches the
	// "disconnected" contract the rest of the extension expects.
	connectionMode = "disconnected";
	console.log("[NativeHost] Port opened, awaiting daemon greeting");
}

/**
 * First message from the daemon proves relay↔daemon connectivity. Only now
 * report "native", reset the reconnect backoff, and register open tabs.
 */
function confirmConnected(): void {
	if (portConfirmed) return;
	portConfirmed = true;
	connectionMode = "native";
	reconnectDelay = RECONNECT_BASE_MS;
	reconnectAttempts = 0;
	console.log("[NativeHost] Connected (daemon confirmed)");

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

interface NativePingMessage {
	type: "ping";
}

// Written by the relay itself (not the daemon) when it cannot reach the
// daemon socket, just before it exits and the port disconnects.
interface NativeErrorMessage {
	type: "error";
	error?: string;
}

type NativeMessage =
	| NativeCommandMessage
	| NativeCaptureScreenshotMessage
	| NativePingMessage
	| NativeErrorMessage;

function handleNativeMessage(msg: NativeMessage): void {
	if (msg.type === "error") {
		// Relay-side failure (daemon down) — not daemon traffic, so it must
		// not confirm the connection. onDisconnect follows and schedules the
		// reconnect.
		console.warn("[NativeHost] Relay error:", msg.error ?? "unknown");
		return;
	}

	// Any daemon-originated message proves the relay↔daemon chain works.
	confirmConnected();

	if (msg.type === "command") {
		const { tabId, payload } = msg;
		void sendCommandToTab(tabId, payload);
		return;
	}

	if (msg.type === "capture_screenshot") {
		void handleCaptureScreenshot(msg);
		return;
	}

	if (msg.type === "ping") {
		// Liveness probe from the daemon; reply so it doesn't reap this
		// relay as stale (see SweepConns in htcli).
		sendToNative({ type: "heartbeat" });
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
						if (parsed.jwt) headers.Authorization = `Bearer ${parsed.jwt}`;
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
			const value = await cdpEvaluate(
				(method, params) =>
					chrome.debugger.sendCommand({ tabId }, method, params),
				expression,
			);
			sendToNative({
				type: "command_result",
				tabId,
				payload: { id, success: true, data: value } as CommandResult,
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

// Actions whose page-side default behavior can start a navigation (link click,
// form submit via Enter, modal button that routes). These get post-action load
// settling; fill/check/select do not navigate on their own (Enter-to-submit
// goes through pressKey).
const SETTLING_ACTIONS = new Set([
	"click",
	"dblclick",
	"rightclick",
	"pressKey",
]);

/**
 * Shape of a `chrome.tabs.onUpdated` listener that this module needs to
 * intercept for tests. Matches the real Chrome signature; the real
 * `chrome.tabs.onUpdated.addListener` accepts a `(tabId, changeInfo, tab)`
 * callback where `changeInfo` is `chrome.tabs.TabChangeInfo` and `tab` is
 * `chrome.tabs.Tab`. Test injection calls the listener with objects that
 * have at least `{ status?, url? }` populated; narrowing is done at the
 * wrapper (`chromeDeps`) so the watcher code can read the full shape.
 */
export type TabUpdateListener = (
	tabId: number,
	changeInfo: chrome.tabs.TabChangeInfo,
	tab: chrome.tabs.Tab,
) => void;

/**
 * Test-injection seam: callers (typically unit tests) provide their own
 * `addListener`/`removeListener` and fire synthetic events into the recorded
 * listeners. The recorded listeners will be the real `TabUpdateListener`
 * signatures; tests only need to populate the fields they exercise
 * (`status`, `url`).
 */
export interface TabWatcherDeps {
	addListener: (fn: TabUpdateListener) => void;
	removeListener: (fn: TabUpdateListener) => void;
}

/**
 * Wait until `tabId` reports a `status: "complete"` update (or a same-document
 * URL change, which has no loading→complete cycle), bounded by `timeoutMs`.
 * Shared by `navigateAndWaitForLoad` and the post-action navigation watcher so
 * there is a single implementation of the "wait for page load" logic.
 * Returns a `cancel` that removes the listener early.
 */
export function waitForTabComplete(
	tabId: number,
	deps: TabWatcherDeps,
	onComplete: (tab: chrome.tabs.Tab) => void,
	onError: (err: Error) => void,
	timeoutMs: number,
): () => void {
	let done = false;
	const listener: TabUpdateListener = (updatedTabId, changeInfo, tab) => {
		if (done || updatedTabId !== tabId) return;
		if (
			changeInfo.status === "complete" ||
			(changeInfo.url !== undefined && tab.status === "complete")
		) {
			done = true;
			clearTimeout(timer);
			deps.removeListener(listener);
			onComplete(tab);
		}
	};
	const timer = setTimeout(() => {
		if (done) return;
		done = true;
		deps.removeListener(listener);
		onError(new Error(`page did not finish loading within ${timeoutMs}ms`));
	}, timeoutMs);
	deps.addListener(listener);
	return () => {
		if (done) return;
		done = true;
		clearTimeout(timer);
		deps.removeListener(listener);
	};
}

export interface NavigationWatcher {
	/** Resolve "completed" if a navigation started and finished, else "none". */
	settle(windowMs: number): Promise<"none" | "completed">;
	/** Stop watching and remove the listener. */
	cancel(): void;
	/**
	 * Record the URL the action was running against. When a `loading` event
	 * arrives, the watcher compares the event's URL to this baseline. A
	 * background navigation on the same URL (ad refresh, polling reload) is
	 * treated as unrelated and the watcher ignores it. Pass the URL
	 * immediately after the action's result returns and before calling
	 * `settle()`. When unset (or when the baseline is "about:blank"), the
	 * watcher falls back to the original "any loading event counts" behavior.
	 */
	setBaseline(url: string | undefined): void;
}

/**
 * Watch a tab for a navigation *triggered by* an action. The listener is
 * attached immediately (before the action is dispatched) so a fast navigation
 * is never missed. After the action's result returns, call `settle(windowMs)`:
 *   - "none" if no `loading` transition was observed within the window;
 *   - "completed" if a `loading` transition was seen and the page then finished
 *     loading (bounded by `loadTimeoutMs`); rejects if the load never completes.
 * `cancel()` removes the listener on any early-exit path.
 *
 * Use `setBaseline(url)` to teach the watcher what URL the action was
 * running against. Once set, a `loading` event whose URL still matches the
 * baseline is treated as an unrelated page reload (e.g. an ad refresh) and
 * does NOT count as an action-triggered navigation.
 */
export function watchForTriggeredNavigation(
	tabId: number,
	deps: TabWatcherDeps,
	loadTimeoutMs = NAV_LOAD_TIMEOUT_MS,
): NavigationWatcher {
	let loadingSeen = false;
	let baselineUrl: string | undefined;
	let finished = false;
	let mainListener: TabUpdateListener | null = null;
	let completeCancel: (() => void) | null = null;
	let settleResolve: ((v: "none" | "completed") => void) | null = null;
	let settleReject: ((e: Error) => void) | null = null;
	let windowTimer: ReturnType<typeof setTimeout> | null = null;

	const removeMain = () => {
		if (mainListener) {
			deps.removeListener(mainListener);
			mainListener = null;
		}
	};

	const beginComplete = () => {
		if (completeCancel) return;
		completeCancel = waitForTabComplete(
			tabId,
			deps,
			() => {
				finished = true;
				if (windowTimer) clearTimeout(windowTimer);
				removeMain();
				settleResolve?.("completed");
			},
			(err) => {
				finished = true;
				if (windowTimer) clearTimeout(windowTimer);
				removeMain();
				settleReject?.(err);
			},
			loadTimeoutMs,
		);
	};

	mainListener = (updatedTabId, changeInfo) => {
		if (finished || updatedTabId !== tabId) return;
		if (changeInfo.status === "loading") {
			// If the page has its own background navigation (ad refresh, polling
			// reload) the new URL is the same as the pre-action URL — ignore it.
			// The action's real navigation will produce a different URL.
			//
			// When changeInfo.url is undefined (the very first loading event
			// for a tab sometimes omits it), we can't correlate, so ignore the
			// event when a baseline is set. The risk is missing a real
			// navigation; the cost is a 500ms settle wait. A baseline being
			// unset is the signal that the caller doesn't have URL info, in
			// which case fall back to the original "any loading event counts"
			// behavior.
			if (baselineUrl !== undefined) {
				if (changeInfo.url === undefined || changeInfo.url === baselineUrl) {
					return;
				}
			}
			loadingSeen = true;
			beginComplete();
		}
	};
	deps.addListener(mainListener);

	const setBaseline = (url: string | undefined) => {
		baselineUrl = url;
	};

	const settle = (windowMs: number): Promise<"none" | "completed"> => {
		return new Promise((resolve, reject) => {
			settleResolve = resolve;
			settleReject = reject;
			windowTimer = setTimeout(() => {
				if (finished) return;
				if (!loadingSeen) {
					finished = true;
					removeMain();
					resolve("none");
				} else {
					// Loading started but hasn't completed yet — keep waiting.
					// Leave the main listener attached so the eventual `complete`
					// event still resolves the promise.
					windowTimer = null;
				}
			}, windowMs);
		});
	};

	const cancel = () => {
		finished = true;
		if (windowTimer) clearTimeout(windowTimer);
		removeMain();
		completeCancel?.();
		if (settleResolve) {
			settleResolve("none");
			settleReject = null;
		}
	};

	return { settle, cancel, setBaseline };
}

/** Real `chrome.tabs.onUpdated` wiring, used by the background relay path.
 *  The cast at the add/remove call site is unavoidable — Chrome's typed
 *  listener uses a different overload than the one this module wants to
 *  pass through — but inside the module we now have the full Tab shape. */
const chromeDeps: TabWatcherDeps = {
	addListener: (fn) => chrome.tabs.onUpdated.addListener(fn),
	removeListener: (fn) => chrome.tabs.onUpdated.removeListener(fn),
};

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
		// Attach the completion watcher before initiating so fast loads aren't
		// missed (fast-load-no-miss). Shares the implementation with the
		// post-action navigation watcher via `waitForTabComplete`.
		const cancel = waitForTabComplete(
			tabId,
			chromeDeps,
			(tab) => resolve(tab),
			reject,
			NAV_LOAD_TIMEOUT_MS,
		);
		// Initiate the navigation; on failure reject (initiate-failure).
		initiateNavigation(tabId, payload).catch((err) => {
			cancel();
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
		// For goBack/goForward, snapshot the URL first so we can detect a
		// silent no-op (chrome.tabs.goBack returns immediately even when
		// there's no history). If no navigation starts within 800ms and the
		// URL is unchanged, fail with an explicit error rather than waiting
		// 25s for a load that will never come.
		const isHistoryAction =
			payload.action === "goBack" || payload.action === "goForward";
		let preUrl: string | undefined;
		if (isHistoryAction) {
			const pre = await chrome.tabs.get(tabId);
			preUrl = pre.url;
		}

		// noopAfterTimeout returns a handle with a `cancel()` that clears the
		// pending 800ms timer and the underlying race promise. Capture the
		// handle so the loser branch of the race is cleaned up the moment the
		// winner resolves — without this, a stray setTimeout fires 800ms after
		// a real navigation completes and calls chrome.tabs.get for no reason.
		const noopHandle = isHistoryAction
			? noopAfterTimeout(
					tabId,
					preUrl as string,
					payload.action as "goBack" | "goForward",
				)
			: null;

		try {
			const tab = await Promise.race<chrome.tabs.Tab>([
				navigateAndWaitForLoad(tabId, payload),
				noopHandle ? noopHandle.promise : new Promise<never>(() => {}),
			]);

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
		} finally {
			noopHandle?.cancel();
		}
	} catch (err) {
		console.error(`[NativeHost] ${payload.action} failed:`, err);
		replyError(
			tabId,
			payload.id,
			err instanceof Error ? err.message : String(err),
		);
	}
}

/**
 * Race a goBack/goForward against a short "did anything change?" timer. If
 * the tab URL is still `preUrl` after 800ms and no error fired, the action
 * was a silent no-op (no history) — fail loudly so callers see a clear
 * "no previous page" message instead of a 25s timeout.
 *
 * Returns a handle with a `cancel()` method the caller MUST invoke after the
 * race resolves. Without it, the setTimeout would still fire 800ms after a
 * real navigation completes, calling chrome.tabs.get for no reason (and
 * potentially logging an error if the tab is closed by then).
 */
function noopAfterTimeout(
	tabId: number,
	preUrl: string,
	action: "goBack" | "goForward",
): { promise: Promise<never>; cancel: () => void } {
	let timer: ReturnType<typeof setTimeout> | null = null;
	const promise = new Promise<never>((_, reject) => {
		timer = setTimeout(async () => {
			timer = null;
			try {
				const post = await chrome.tabs.get(tabId);
				if (post.url === preUrl) {
					reject(
						new Error(
							action === "goBack"
								? "No previous page in this tab's history"
								: "No forward page in this tab's history",
						),
					);
				}
			} catch (err) {
				reject(err);
			}
		}, 800);
	});
	return {
		promise,
		cancel: () => {
			if (timer !== null) {
				clearTimeout(timer);
				timer = null;
			}
		},
	};
}

/**
 * Dispatch a trusted (CDP) input command, injecting the content script and
 * retrying once if it isn't ready yet — mirroring the synthetic path's
 * inject-and-retry below. The debugger attaches/detaches inside `dispatchCdpInput`,
 * so a navigation triggered by the action never collides with the attach.
 */
async function dispatchCdpInputWithRetry(
	tabId: number,
	payload: Command,
): Promise<CommandResult> {
	try {
		return await dispatchCdpInput(tabId, payload);
	} catch (err) {
		if (!(err instanceof ContentScriptNotReadyError)) throw err;
		// Content script not ready — inject it and retry once.
		try {
			const manifest = chrome.runtime.getManifest();
			const contentScriptFile =
				manifest.content_scripts?.[0]?.js?.[0] ?? "assets/chunk-DjFz53LA.js";
			await chrome.scripting.executeScript({
				target: { tabId },
				files: [contentScriptFile],
			});
			await new Promise((r) => setTimeout(r, 300));
		} catch {
			throw new Error("tab not available");
		}
		return await dispatchCdpInput(tabId, payload);
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
	if (payload.action === "getReadyTabs") {
		// Background-handled: report which tabs have a live content script
		// (the sidepanel's "Connected Tabs" view), for diagnostics via htcli.
		try {
			const tabs = readyTabsProvider ? await readyTabsProvider() : [];
			sendToNative({
				type: "command_result",
				tabId,
				payload: { id: payload.id, success: true, data: tabs } as CommandResult,
			});
		} catch (err) {
			replyError(
				tabId,
				payload.id,
				err instanceof Error ? err.message : String(err),
			);
		}
		return;
	}
	if (payload.action === "debuggerEval") {
		await handleDebuggerEval(tabId, payload);
		return;
	}
	// On Chrome, route `evaluate` through CDP so the script runs in the page's
	// main world (not the content script's isolated world) and isTrusted, with
	// no `new Function` CSP issues. On Firefox `chrome.debugger` is undefined,
	// so the content-script `new Function` path handles it there.
	if (payload.action === "evaluate" && typeof chrome.debugger !== "undefined") {
		await handleDebuggerEval(tabId, payload);
		return;
	}

	// For actions whose default behavior can start a navigation, watch the tab
	// for a triggered load. The watcher is created once, before the first
	// sendMessage / CDP dispatch attempt, so a fast navigation is never missed.
	const settling = SETTLING_ACTIONS.has(payload.action);
	const watcher = settling
		? watchForTriggeredNavigation(tabId, chromeDeps, NAV_LOAD_TIMEOUT_MS)
		: null;

	const start = Date.now();

	// ── Trusted (CDP) input on Chrome ───────────────────────────────
	// On Chrome (`chrome.debugger` exists) click/dblclick/rightclick/pressKey/type
	// are dispatched as trusted CDP input: the content script prepares the
	// element (wait actionable, scroll, focus) and reports coords, then we attach
	// the debugger and dispatch. The debugger detaches *inside* the dispatcher,
	// before the settle wait below begins — a navigation would otherwise kill the
	// attach. Firefox has no `chrome.debugger`, so we fall through to the synthetic
	// content-script path (which is also upgraded to emit pointer events).
	const cdpDispatchable =
		typeof chrome.debugger !== "undefined" &&
		CDP_INPUT_ACTIONS.has(payload.action);
	if (cdpDispatchable) {
		let cdpResult: CommandResult;
		try {
			cdpResult = await dispatchCdpInputWithRetry(tabId, payload);
		} catch (err) {
			watcher?.cancel();
			replyError(
				tabId,
				payload.id,
				err instanceof Error ? err.message : String(err),
			);
			return;
		}
		if (watcher) {
			// Hand the watcher the pre-action URL so it can ignore background
			// navigations on the same URL (ad refresh, polling reload) that
			// would otherwise hang settle for the full 25s.
			watcher.setBaseline(cdpResult.pageInfo?.url);
			try {
				const outcome = await watcher.settle(500);
				if (outcome === "completed") {
					// Include the load time in the reported duration.
					cdpResult = { ...cdpResult, duration: Date.now() - start };
				}
			} catch (err) {
				replyError(
					tabId,
					payload.id,
					`${payload.action} started a navigation that never finished loading: ${
						err instanceof Error ? err.message : String(err)
					}`,
				);
				return;
			}
		}
		sendToNative({
			type: "command_result",
			tabId,
			payload: stripScreenshot(cdpResult),
		});
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
			watcher?.cancel();
			console.warn("[NativeHost] content script injection failed:", err);
			// Surface the real injection error — "Missing host permission",
			// restricted domain, etc. — instead of a generic message, so the
			// failure is diagnosable from the htcli side.
			replyError(
				tabId,
				payload.id,
				`tab not available (content script injection failed: ${
					err instanceof Error ? err.message : String(err)
				})`,
			);
			return;
		}
		result = await sendMsg();
	}

	if (result === null) {
		watcher?.cancel();
		replyError(
			tabId,
			payload.id,
			"tab not available (content script did not respond after injection)",
		);
		return;
	}

	// Settle: if the action triggered a navigation, wait for it to finish
	// loading before reporting success.
	if (watcher) {
		// Hand the watcher the pre-action URL so it can ignore background
		// navigations on the same URL (ad refresh, polling reload) that
		// would otherwise hang settle for the full 25s.
		watcher.setBaseline(result.pageInfo?.url);
		try {
			const outcome = await watcher.settle(500);
			if (outcome === "completed") {
				// Include the load time in the reported duration.
				result = { ...result, duration: Date.now() - start };
			}
		} catch (err) {
			replyError(
				tabId,
				payload.id,
				`${payload.action} started a navigation that never finished loading: ${
					err instanceof Error ? err.message : String(err)
				}`,
			);
			return;
		}
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
