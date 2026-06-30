/**
 * Command Executor
 * Executes remote control commands on the page.
 * Receives commands from the content script message handler and performs actions.
 *
 * SECURITY NOTE: The "evaluate" action executes arbitrary JavaScript in the page context.
 * This is by design for browser automation, but callers must ensure proper authentication
 * is in place. The server enforces IP whitelist + bearer token auth by default.
 */

import type {
	BoundingBox,
	Command,
	CommandResult,
	PageInfo,
	RemoteElementInfo,
	TargetSelector,
} from "../types/commands";
import {
	findElement,
	findElementInfo,
	getElementInfo,
	waitForElement,
} from "./elementFinder";
import { generateXPath } from "./xpathGenerator";

// ─── Main Entry Point ─────────────────────────────────────────────

/**
 * Execute a command and return a result.
 */
export async function executeCommand(command: Command): Promise<CommandResult> {
	const start = Date.now();

	try {
		const data = await executeAction(command);
		return {
			id: command.id,
			success: true,
			data,
			duration: Date.now() - start,
			pageInfo: getPageInfo(),
		};
	} catch (error) {
		return {
			id: command.id,
			success: false,
			error: error instanceof Error ? error.message : String(error),
			duration: Date.now() - start,
			pageInfo: getPageInfo(),
		};
	}
}

// ─── Action Router ─────────────────────────────────────────────────

/**
 * Assert target is present (for actions that require it).
 * Throws a descriptive error if missing.
 */
function requireTarget(
	target: TargetSelector | undefined,
	action: string,
): TargetSelector {
	if (!target) {
		throw new Error(
			`Action "${action}" requires a "target" selector in the command`,
		);
	}
	return target;
}

/**
 * Assert value is present (for actions that require it).
 * Throws a descriptive error if missing.
 */
function requireValue(value: string | undefined, action: string): string {
	if (value === undefined) {
		throw new Error(`Action "${action}" requires a "value" in the command`);
	}
	return value;
}

async function executeAction(command: Command): Promise<unknown> {
	const { action, target, value, options } = command;

	// At this point, target/value have been validated for actions that need them.
	// requireTarget/requireValue provide type-safe non-null returns.
	switch (action) {
		// ─── Finding / Inspection ─────────────────────────────────────
		case "find":
			return handleFind(requireTarget(target, action));
		case "findAll":
			return handleFindAll(requireTarget(target, action));
		case "wait":
			return handleWait(
				requireTarget(target, action),
				options?.timeout as number,
			);
		case "isVisible":
			return handleIsVisible(requireTarget(target, action));
		case "isEnabled":
			return handleIsEnabled(requireTarget(target, action));
		case "getValue":
			return handleGetValue(requireTarget(target, action));
		case "getAttribute":
			return handleGetAttribute(
				requireTarget(target, action),
				options?.attribute as string,
			);
		case "getText":
			return handleGetText(requireTarget(target, action));
		case "getHTML":
			return handleGetHTML(
				requireTarget(target, action),
				options?.outer as boolean,
			);
		case "getOuterHTML":
			return handleGetHTML(requireTarget(target, action), true);
		case "getBoundingBox":
			return handleGetBoundingBox(requireTarget(target, action));
		case "getComputedStyle":
			return handleGetComputedStyle(
				requireTarget(target, action),
				options?.property as string,
			);
		case "getPageInfo":
			return getPageInfo();
		case "xpath":
			return handleXPath(requireTarget(target, action));

		// ─── Interaction ──────────────────────────────────────────────
		case "click":
			return handleClick(
				requireTarget(target, action),
				options?.button as string,
				options?.count as number,
			);
		case "dblclick":
			return handleClick(requireTarget(target, action), "left", 2);
		case "rightclick":
			return handleClick(requireTarget(target, action), "right", 1);
		case "hover":
			return handleHover(requireTarget(target, action));
		case "focus":
			return handleFocus(requireTarget(target, action));
		case "blur":
			return handleBlur(requireTarget(target, action));
		case "scrollTo":
			return handleScrollTo(requireTarget(target, action));
		case "fill":
			return handleFill(
				requireTarget(target, action),
				requireValue(value, action),
			);
		case "type":
			return handleType(
				requireTarget(target, action),
				requireValue(value, action),
			);
		case "clear":
			return handleClear(requireTarget(target, action));
		case "select":
			return handleSelect(
				requireTarget(target, action),
				requireValue(value, action),
			);
		case "check":
			return handleCheck(requireTarget(target, action), true);
		case "uncheck":
			return handleCheck(requireTarget(target, action), false);
		case "pressKey":
			return handlePressKey(
				requireTarget(target, action),
				requireValue(value, action),
			);
		case "selectText":
			return handleSelectText(requireTarget(target, action));

		// ─── Navigation ───────────────────────────────────────────────
		case "navigate":
			return handleNavigate(requireValue(value, action));
		case "reload":
			return handleReload();
		case "goBack":
			return handleGoBack();
		case "goForward":
			return handleGoForward();

		// ─── Screenshot ───────────────────────────────────────────────
		case "screenshot":
			return handleScreenshot(target);

		// ─── Script Execution ─────────────────────────────────────────
		// SECURITY: This action executes arbitrary JavaScript in the page context.
		// It is intended for browser automation use cases where the caller is trusted.
		// The server enforces authentication (IP whitelist + bearer token) before
		// forwarding commands to the extension.
		case "evaluate":
			return handleEvaluate(requireValue(value, action));

		case "fetch":
			return handleFetchViaBackground(requireValue(value, action), options);

		case "printToPDF":
			return handlePrintToPDF(options?.tabId as number | undefined);

		// ─── Highlight ────────────────────────────────────────────────
		case "highlight":
			return handleHighlight(requireTarget(target, action));
		case "unhighlight":
			return handleUnhighlight();

		// ─── Tab Management ──────────────────────────────────────────
		case "listTabs":
			return handleListTabs();
		case "getTabInfo":
			return handleGetTabInfo(options?.tabId as number);
		case "switchTab":
			return handleSwitchTab(requireValue(value, action));
		case "getSessionStorage":
			return sessionStorage.getItem(value || "eReceiptData");
		case "getLocalStorage":
			return localStorage.getItem(requireValue(value, action));
		case "fetchInPage":
			return handleFetchInPage(requireValue(value, action), options);
		case "fetchViaDOM":
			return handleFetchInPage(requireValue(value, action), options);
		case "fetchFromCS":
			return handleFetchFromCS(requireValue(value, action), options);
		case "openTab":
			return handleOpenTab(options as { url: string; sessionData?: string });
		case "closeTab":
			return handleCloseTab((options?.tabId as number) || Number(value));
		case "cdpNavigate":
			return handleCdpNavigate(
				options?.tabId as number,
				requireValue(value, action),
			);

		default:
			throw new Error(`Unknown action: ${action}`);
	}
}

// ─── Find / Inspection Handlers ────────────────────────────────────

function handleFind(target: TargetSelector): RemoteElementInfo | null {
	const element = findElement(target);
	if (!element) return null;
	return getElementInfo(element);
}

function handleFindAll(target: TargetSelector): RemoteElementInfo[] {
	return findElementInfo(target);
}

async function handleWait(
	target: TargetSelector,
	timeout?: number,
): Promise<RemoteElementInfo | null> {
	const element = await waitForElement(target, timeout ?? 5000);
	if (!element) return null;
	return getElementInfo(element);
}

function handleIsVisible(target: TargetSelector): boolean {
	const element = findElement(target);
	if (!element) return false;
	const info = getElementInfo(element);
	return info.visible ?? false;
}

function handleIsEnabled(target: TargetSelector): boolean {
	const element = findElement(target);
	if (!element) return false;
	const info = getElementInfo(element);
	return info.enabled ?? true;
}

function handleGetValue(target: TargetSelector): string {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");

	if (
		element instanceof HTMLInputElement ||
		element instanceof HTMLTextAreaElement
	) {
		return element.value;
	}
	if (element instanceof HTMLSelectElement) {
		return element.value;
	}
	if (element instanceof HTMLElement) {
		return element.textContent || "";
	}
	throw new Error("Cannot get value from this element type");
}

function handleGetAttribute(
	target: TargetSelector,
	attribute: string,
): string | null {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");
	return element.getAttribute(attribute);
}

function handleGetText(target: TargetSelector): string {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");
	return element.textContent || "";
}

function handleGetHTML(target: TargetSelector, outer = false): string {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");
	if (outer) return element.outerHTML;
	return element.innerHTML;
}

function handleGetBoundingBox(target: TargetSelector): BoundingBox | null {
	const element = findElement(target);
	if (!element) return null;
	const rect = element.getBoundingClientRect();
	return {
		x: rect.x,
		y: rect.y,
		width: rect.width,
		height: rect.height,
		top: rect.top,
		bottom: rect.bottom,
		left: rect.left,
		right: rect.right,
	};
}

function handleGetComputedStyle(
	target: TargetSelector,
	property: string,
): string | null {
	const element = findElement(target);
	if (!element) return null;
	const style = window.getComputedStyle(element);
	return style.getPropertyValue(property);
}

function handleXPath(target: TargetSelector): string | null {
	const element = findElement(target);
	if (!element) return null;
	return generateXPath(element);
}

// ─── Interaction Handlers ──────────────────────────────────────────

function handleClick(target: TargetSelector, button = "left", count = 1): void {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");

	const rect = element.getBoundingClientRect();
	const x = rect.left + rect.width / 2;
	const y = rect.top + rect.height / 2;

	const eventInit: MouseEventInit = {
		bubbles: true,
		cancelable: true,
		view: window,
		button: button === "right" ? 2 : button === "middle" ? 1 : 0,
		clientX: x,
		clientY: y,
	};

	// Dispatch the full mouse event sequence (matching browser event order)
	const singleClickSequence = [
		"mouseover",
		"mouseenter",
		"mousedown",
		"mouseup",
		"click",
	];

	for (let i = 0; i < count; i++) {
		for (const eventType of singleClickSequence) {
			(element as HTMLElement).dispatchEvent(
				new MouseEvent(eventType, eventInit),
			);
		}
	}

	// dblclick fires after both click sequences, matching the DOM Level 3 event order
	if (count === 2) {
		(element as HTMLElement).dispatchEvent(
			new MouseEvent("dblclick", eventInit),
		);
	}

	// Scroll element into view if needed
	element.scrollIntoView({ behavior: "smooth", block: "nearest" });
}

function handleHover(target: TargetSelector): void {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");

	const rect = element.getBoundingClientRect();
	const x = rect.left + rect.width / 2;
	const y = rect.top + rect.height / 2;

	const eventInit: MouseEventInit = {
		bubbles: true,
		cancelable: true,
		view: window,
		clientX: x,
		clientY: y,
	};

	(element as HTMLElement).dispatchEvent(
		new MouseEvent("mouseover", eventInit),
	);
	(element as HTMLElement).dispatchEvent(
		new MouseEvent("mouseenter", eventInit),
	);
}

function handleFocus(target: TargetSelector): void {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");
	(element as HTMLElement).focus();
}

function handleBlur(target: TargetSelector): void {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");
	(element as HTMLElement).blur();
}

function handleScrollTo(target: TargetSelector): void {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");
	element.scrollIntoView({ behavior: "smooth", block: "center" });
}

function handleFill(target: TargetSelector, value: string): void {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");

	if (
		element instanceof HTMLInputElement ||
		element instanceof HTMLTextAreaElement ||
		element instanceof HTMLSelectElement
	) {
		// Focus first
		(element as HTMLInputElement).focus();

		// Set value
		if (element instanceof HTMLSelectElement) {
			element.value = value;
		} else {
			// Use native setter to bypass React's synthetic event system
			const nativeSetter = Object.getOwnPropertyDescriptor(
				element instanceof HTMLTextAreaElement
					? HTMLTextAreaElement.prototype
					: HTMLInputElement.prototype,
				"value",
			)?.set;
			if (nativeSetter) {
				nativeSetter.call(element, value);
			} else {
				element.value = value;
			}
		}

		// Dispatch events to trigger framework handlers
		element.dispatchEvent(new Event("input", { bubbles: true }));
		element.dispatchEvent(new Event("change", { bubbles: true }));
		element.dispatchEvent(new Event("blur", { bubbles: true }));
	} else {
		throw new Error("Element is not a form input");
	}
}

function handleType(target: TargetSelector, value: string): void {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");

	if (
		element instanceof HTMLInputElement ||
		element instanceof HTMLTextAreaElement
	) {
		(element as HTMLInputElement).focus();

		// Type each character
		for (const char of value) {
			const keyDownEvent = new KeyboardEvent("keydown", {
				bubbles: true,
				key: char,
				code: `Key${char.toUpperCase()}`,
			});
			const keyPressEvent = new KeyboardEvent("keypress", {
				bubbles: true,
				key: char,
				code: `Key${char.toUpperCase()}`,
			});
			const inputEvent = new InputEvent("beforeinput", {
				bubbles: true,
				data: char,
				inputType: "insertText",
			});
			const afterInputEvent = new InputEvent("input", {
				bubbles: true,
				data: char,
				inputType: "insertText",
			});
			const keyUpEvent = new KeyboardEvent("keyup", {
				bubbles: true,
				key: char,
				code: `Key${char.toUpperCase()}`,
			});

			(element as HTMLElement).dispatchEvent(keyDownEvent);
			(element as HTMLElement).dispatchEvent(keyPressEvent);
			(element as HTMLElement).dispatchEvent(inputEvent);
			// Actually set the value
			(element as HTMLInputElement).value += char;
			(element as HTMLElement).dispatchEvent(afterInputEvent);
			(element as HTMLElement).dispatchEvent(keyUpEvent);
		}

		// Dispatch change event
		element.dispatchEvent(new Event("change", { bubbles: true }));
		element.dispatchEvent(new Event("blur", { bubbles: true }));
	} else {
		throw new Error("Element is not a text input");
	}
}

function handleClear(target: TargetSelector): void {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");

	if (
		element instanceof HTMLInputElement ||
		element instanceof HTMLTextAreaElement
	) {
		(element as HTMLInputElement).focus();
		(element as HTMLInputElement).value = "";
		element.dispatchEvent(new Event("input", { bubbles: true }));
		element.dispatchEvent(new Event("change", { bubbles: true }));
	} else {
		throw new Error("Element is not a text input");
	}
}

function handleSelect(target: TargetSelector, value: string): void {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");

	if (element instanceof HTMLSelectElement) {
		(element as HTMLSelectElement).focus();
		// Use native setter to bypass React's synthetic event system
		const nativeSetter = Object.getOwnPropertyDescriptor(
			HTMLSelectElement.prototype,
			"value",
		)?.set;
		if (nativeSetter) {
			nativeSetter.call(element, value);
		} else {
			element.value = value;
		}
		element.dispatchEvent(new Event("input", { bubbles: true }));
		element.dispatchEvent(new Event("change", { bubbles: true }));
	} else {
		throw new Error("Element is not a select");
	}
}

function handleCheck(target: TargetSelector, checked: boolean): void {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");

	if (element instanceof HTMLInputElement) {
		(element as HTMLInputElement).focus();
		// Use native setter to bypass React's synthetic event system
		const nativeSetter = Object.getOwnPropertyDescriptor(
			HTMLInputElement.prototype,
			"checked",
		)?.set;
		if (nativeSetter) {
			nativeSetter.call(element, checked);
		} else {
			element.checked = checked;
		}
		element.dispatchEvent(new Event("change", { bubbles: true }));
	} else {
		throw new Error("Element is not a checkbox/radio input");
	}
}

function handlePressKey(target: TargetSelector, key: string): void {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");

	(element as HTMLElement).focus();

	const keyDownEvent = new KeyboardEvent("keydown", {
		bubbles: true,
		key,
		code: `Key${key}`,
	});
	const keyUpEvent = new KeyboardEvent("keyup", {
		bubbles: true,
		key,
		code: `Key${key}`,
	});

	(element as HTMLElement).dispatchEvent(keyDownEvent);
	(element as HTMLElement).dispatchEvent(keyUpEvent);
}

function handleSelectText(target: TargetSelector): void {
	const element = findElement(target);
	if (!element) throw new Error("Element not found");

	if (
		element instanceof HTMLInputElement ||
		element instanceof HTMLTextAreaElement
	) {
		(element as HTMLInputElement).select();
	} else {
		// Select all text in element
		const selection = window.getSelection();
		if (selection) {
			const range = document.createRange();
			range.selectNodeContents(element);
			selection.removeAllRanges();
			selection.addRange(range);
		}
	}
}

// ─── Navigation Handlers ───────────────────────────────────────────

function handleNavigate(url: string): void {
	if (!url) throw new Error("URL is required");
	window.location.href = url;
}

function handleReload(): void {
	window.location.reload();
}

function handleGoBack(): void {
	window.history.back();
}

function handleGoForward(): void {
	window.history.forward();
}

// ─── Screenshot Handler ────────────────────────────────────────────

function handleScreenshot(_target?: TargetSelector): string {
	// Screenshots are handled by the background script
	// This is a placeholder that signals the background to capture
	// The actual capture happens in the background via message passing
	return "screenshot_requested";
}

// ─── Script Execution Handler ──────────────────────────────────────

/**
 * Execute arbitrary JavaScript in the page context.
 *
 * SECURITY WARNING: This function evaluates arbitrary code with full page access.
 * It is intended for trusted browser automation scenarios. The server enforces
 * authentication (IP whitelist + bearer token) before forwarding commands.
 * Do not expose this endpoint to untrusted callers.
 */
function handleEvaluate(script: string): unknown {
	if (!script) throw new Error("Script is required");

	// SECURITY: Use Function constructor for indirect eval in global scope.
	// This executes arbitrary code with full page access.
	const fn = new Function(`return (${script})`);
	return fn();
}

async function handlePrintToPDF(targetTabId?: number): Promise<unknown> {
	let tabId = targetTabId;
	if (!tabId) {
		tabId = await new Promise<number>((resolve) => {
			chrome.runtime.sendMessage(
				{ type: "GET_TAB_ID" },
				(resp: { tabId: number }) => resolve(resp.tabId),
			);
		});
	}
	const response = await chrome.runtime.sendMessage({
		type: "PRINT_TO_PDF",
		tabId,
	});
	if (!response?.ok) throw new Error(response?.error || "printToPDF failed");
	return response.data;
}

const AIA_API_KEY = "50efbade-11e8-4169-abc3-e84e1b4c561b";

// Fetch from the content script's isolated world — shares the page's cookie jar
// (credentials: "include" sends session cookies), reads JWT from localStorage.
// Returns { status, contentType, base64 } so binary PDFs are transferred safely.
async function handleFetchFromCS(
	url: string,
	options?: Record<string, unknown>,
): Promise<unknown> {
	const method = (options?.method as string) || "POST";
	const headers: Record<string, string> = {
		"Content-Type": "application/json",
		Accept: "application/json",
		"X-Channel-ID": "MYP_WEB",
		"X-Gateway-APIKey": AIA_API_KEY,
		"Content-Language": "en",
		...((options?.headers as Record<string, string>) ?? {}),
	};
	// Read JWT from page's localStorage (content scripts share it)
	try {
		const raw = localStorage.getItem("OAOP_LOGINDATA");
		if (raw) {
			const p = JSON.parse(raw) as { jwt?: string };
			if (p.jwt) headers["Authorization"] = `Bearer ${p.jwt}`;
		}
	} catch {
		// intentionally not logged: failures are benign
	}
	const body = options?.body;
	const resp = await fetch(url, {
		method,
		headers,
		credentials: "include",
		body: body !== undefined ? JSON.stringify(body) : undefined,
	});
	const status = resp.status;
	const contentType = resp.headers.get("content-type") || "";
	if (!resp.ok) {
		const errText = await resp.text();
		throw new Error(`HTTP ${status}: ${errText.slice(0, 400)}`);
	}
	// Base64-encode for binary-safe transfer
	const buf = await resp.arrayBuffer();
	const bytes = new Uint8Array(buf);
	let bin = "";
	for (let i = 0; i < bytes.length; i += 8192) {
		bin += String.fromCharCode(...bytes.subarray(i, Math.min(i + 8192, bytes.length)));
	}
	return { status, contentType, base64: btoa(bin) };
}

// Runs fetch in the PAGE's JS context (MAIN world) via <script> injection,
// so the request goes out with Origin: https://www.aia.com.my instead of the
// extension origin. Requires the page to have no CSP blocking inline scripts.
const FETCH_IN_PAGE_ALLOWED_HOSTS = new Set(["api.aia.com.my", "www.aia.com.my"]);

async function handleFetchInPage(
	url: string,
	options?: Record<string, unknown>,
): Promise<unknown> {
	const parsed = new URL(url);
	if (
		parsed.protocol !== "https:" ||
		!FETCH_IN_PAGE_ALLOWED_HOSTS.has(parsed.hostname)
	) {
		throw new Error(`fetchInPage: URL not in allowlist: ${parsed.hostname}`);
	}
	const jwtRaw = localStorage.getItem("OAOP_LOGINDATA");
	let jwt = "";
	try {
		const parsed = JSON.parse(jwtRaw || "{}") as { jwt?: string };
		jwt = parsed.jwt || "";
	} catch {
		// intentionally not logged: benign
	}

	const headers: Record<string, string> = {
		"Content-Type": "application/json",
		Accept: "application/json",
		"X-Channel-ID": "MYP_WEB",
		"X-Gateway-APIKey": AIA_API_KEY,
		"Content-Language": "en",
		...(jwt ? { Authorization: `Bearer ${jwt}` } : {}),
		...((options?.headers as Record<string, string>) ?? {}),
	};

	const msgId = `__aip_${Date.now()}_${Math.random().toString(36).slice(2)}`;

	return new Promise<unknown>((resolve, reject) => {
		const timeout = setTimeout(() => {
			window.removeEventListener("message", listener);
			reject(new Error("fetchInPage timeout after 30s"));
		}, 30000);

		function listener(event: MessageEvent) {
			if (
				event.source !== window ||
				(event.data as { __id?: string })?.__id !== msgId
			)
				return;
			clearTimeout(timeout);
			window.removeEventListener("message", listener);
			const d = event.data as { ok: boolean; data?: unknown; error?: string };
			if (!d.ok) reject(new Error(d.error ?? "fetchInPage failed"));
			else resolve(d.data);
		}
		window.addEventListener("message", listener);

		const fetchBody =
			options?.body !== undefined ? JSON.stringify(options.body) : undefined;
		const scriptCode = `(async () => {
  const _id = ${JSON.stringify(msgId)};
  try {
    const r = await fetch(${JSON.stringify(url)}, {
      method: ${JSON.stringify((options?.method as string) || "POST")},
      headers: ${JSON.stringify(headers)},
      credentials: "include",
      body: ${fetchBody !== undefined ? JSON.stringify(fetchBody) : "undefined"},
    });
    const text = await r.text();
    if (!r.ok) throw new Error("HTTP " + r.status + ": " + text.slice(0, 200));
    let data; try { data = JSON.parse(text); } catch { data = text; }
    window.postMessage({ __id: _id, ok: true, data }, "*");
  } catch(e) {
    window.postMessage({ __id: _id, ok: false, error: e.message }, "*");
  }
})();`;

		const script = document.createElement("script");
		script.textContent = scriptCode;
		document.head.appendChild(script);
		document.head.removeChild(script);
	});
}

async function handleFetchViaBackground(
	url: string,
	options?: Record<string, unknown>,
): Promise<unknown> {
	// Build default headers; auto-inject AIA JWT only for trusted AIA API origins
	const AIA_API_ORIGINS = new Set([
		"https://api.aia.com.my",
		"https://myaia.aia.com.my",
	]);
	const defaultHeaders: Record<string, string> = {
		"Content-Type": "application/json",
		Accept: "application/json",
		"X-Channel-ID": "MYP_WEB",
		"X-Request-ID": `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
		"Content-Language": "en",
	};
	try {
		const targetOrigin = new URL(url).origin;
		if (AIA_API_ORIGINS.has(targetOrigin)) {
			const raw = localStorage.getItem("OAOP_LOGINDATA");
			if (raw) {
				const parsed = JSON.parse(raw) as { jwt?: string };
				if (parsed.jwt)
					defaultHeaders["Authorization"] = `Bearer ${parsed.jwt}`;
			}
		}
	} catch {
		// ignore — invalid URL or no AIA login data
	}

	const response = await chrome.runtime.sendMessage({
		type: "FETCH_URL",
		url,
		method: (options?.method as string) || "POST",
		headers: (options?.headers as Record<string, string>) ?? defaultHeaders,
		body: options?.body,
	});
	if (!response?.ok)
		throw new Error(
			response?.error || `Fetch failed: HTTP ${response?.status}`,
		);
	return response.data;
}

// ─── Highlight Handlers ────────────────────────────────────────────

function handleHighlight(target: TargetSelector): RemoteElementInfo | null {
	const element = findElement(target);
	if (!element) return null;

	// Dispatch custom event for the highlighter to pick up
	window.dispatchEvent(
		new CustomEvent("how-to-recorder:highlight", {
			detail: { element },
		}),
	);

	return getElementInfo(element);
}

function handleUnhighlight(): void {
	window.dispatchEvent(new CustomEvent("how-to-recorder:unhighlight"));
}

// ─── Page Info ─────────────────────────────────────────────────────

function getPageInfo(): PageInfo {
	return {
		url: window.location.href,
		title: document.title,
		domain: window.location.hostname,
		scrollX: window.scrollX,
		scrollY: window.scrollY,
		viewportWidth: window.innerWidth,
		viewportHeight: window.innerHeight,
		documentHeight: document.documentElement.scrollHeight,
		documentWidth: document.documentElement.scrollWidth,
	};
}

async function handleOpenTab(opts: {
	url: string;
	sessionData?: string;
}): Promise<{ tabId: number }> {
	const response = await chrome.runtime.sendMessage({
		type: "OPEN_TAB",
		url: opts.url,
		sessionData: opts.sessionData,
	});
	if (!response?.ok) throw new Error(response?.error || "openTab failed");
	return { tabId: response.tabId };
}

async function handleCloseTab(tabId: number): Promise<void> {
	const response = await chrome.runtime.sendMessage({
		type: "CLOSE_TAB",
		tabId,
	});
	if (!response?.ok) throw new Error(response?.error || "closeTab failed");
}

async function handleCdpNavigate(tabId: number, url: string): Promise<void> {
	const response = await chrome.runtime.sendMessage({
		type: "CDP_NAVIGATE",
		tabId,
		url,
	});
	if (!response?.ok) throw new Error(response?.error || "cdpNavigate failed");
}

// ─── Tab Management ──────────────────────────────────────────────
// These commands require Chrome extension APIs not available in content scripts.
// They delegate to the background service worker via chrome.runtime.sendMessage().

/**
 * List all connected browser tabs.
 */
async function handleListTabs(): Promise<
	Array<{ id: number; url: string; title: string; active: boolean }>
> {
	const response = await chrome.runtime.sendMessage({ type: "GET_TABS_INFO" });
	if (!response?.success) {
		throw new Error(response?.error || "Failed to list tabs");
	}
	return response.tabs;
}

/**
 * Get info about a specific tab by ID.
 * If no tabId is provided, returns info about the current tab.
 */
async function handleGetTabInfo(
	tabId?: number,
): Promise<{ id: number; url: string; title: string; active: boolean }> {
	// If no tabId specified, get the current tab's info from background
	if (tabId === undefined) {
		const response = await chrome.runtime.sendMessage({
			type: "GET_CURRENT_TAB_ID",
		});
		if (!response?.tabId) {
			throw new Error("No current tab available");
		}
		tabId = response.tabId;
	}

	// Get all tabs and find the one we want
	const tabsResponse = await chrome.runtime.sendMessage({
		type: "GET_TABS_INFO",
	});
	if (!tabsResponse?.success) {
		throw new Error(tabsResponse?.error || "Failed to get tab info");
	}

	const tab = tabsResponse.tabs.find((t: { id: number }) => t.id === tabId);
	if (!tab) {
		throw new Error(`Tab ${tabId} not found`);
	}
	return tab;
}

/**
 * Switch to (activate) a tab by ID.
 * The value parameter should be the tab ID as a string.
 */
async function handleSwitchTab(
	tabIdStr: string,
): Promise<{ success: boolean }> {
	const tabId = Number(tabIdStr);
	if (Number.isNaN(tabId)) {
		throw new Error(`Invalid tab ID: ${tabIdStr}`);
	}
	const response = await chrome.runtime.sendMessage({
		type: "SWITCH_TAB",
		tabId,
	});
	if (!response?.success) {
		throw new Error(response?.error || `Failed to switch to tab ${tabId}`);
	}
	return { success: true };
}

// ─── Page Info ─────────────────────────────────────────────────────
