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
	CommandAction,
	CommandResult,
	PageInfo,
	RemoteElementInfo,
	TargetSelector,
} from "../types/commands";
import { AIA_API_KEY } from "../utils/aiaConfig";
import { resolveKey } from "../utils/keyMap";
import {
	findElement,
	findElementInfo,
	getElementInfo,
	waitForActionableElement,
	waitForElement,
} from "./elementFinder";
import { generateXPath } from "./xpathGenerator";

/**
 * Internal action used by the server to route a user's `evaluate` request
 * through the content script → background → CDP path. Mirrors the matching
 * `evaluateViaCdp` entry on the server side (`server/index.ts`). Using a
 * typed const here so a future rename on either side fails the TypeScript
 * build rather than silently breaking the case dispatch.
 */
const EVALUATE_VIA_CDP: CommandAction = "evaluateViaCdp";

/**
 * Actions relayed to the background for trusted (CDP) input on Chrome.
 * These are the only interaction actions whose default behavior benefits
 * from trusted events. `scrollTo`/`fill`/etc. keep their synthetic path
 * everywhere (they already produce correct results without `isTrusted`).
 */
const CDP_INPUT_RELAY_ACTIONS = new Set<CommandAction>([
	"click",
	"dblclick",
	"rightclick",
	"pressKey",
	"type",
]);

/**
 * Whether the trusted (CDP) input path is available. On Chrome `chrome.debugger`
 * exists; on Firefox (and other browsers without it) it does not, so we keep the
 * synthetic path there. Guarded so an undeclared `chrome` (unit-test realm) is
 * also treated as unavailable.
 */
function cdpInputAvailable(): boolean {
	return (
		typeof chrome !== "undefined" && typeof chrome.debugger !== "undefined"
	);
}

/**
 * Relay a click/pressKey/type command to the background, which owns the
 * `chrome.debugger` connection and dispatches trusted CDP input. Returns the
 * background's `CommandResult`, or `null` when the background reports that CDP
 * input is unsupported (so the caller can fall back to the synthetic path).
 */
async function relayCdpInput(command: Command): Promise<CommandResult | null> {
	const response = await new Promise<CommandResult | null>((resolve) => {
		chrome.runtime.sendMessage(
			{ type: "CDP_INPUT", command },
			(result: CommandResult) => {
				if (chrome.runtime.lastError) {
					resolve(null);
					return;
				}
				resolve(result ?? null);
			},
		);
	});
	if (!response) return null;
	// The background signals "unsupported" (e.g. Firefox without the debugger)
	// with this sentinel in the error string — fall back to synthetic input.
	if (
		!response.success &&
		typeof response.error === "string" &&
		response.error.includes("CDP_INPUT_UNSUPPORTED")
	) {
		return null;
	}
	return response;
}

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

/**
 * Resolve the wait budget (ms) for an interaction action from its options.
 * Defaults to 5000ms (matching `waitForActionableElement`) and is capped at
 * 20000ms so a single action can never consume the whole transport budget.
 */
function waitTimeout(options?: Command["options"]): number {
	const t = options?.timeout;
	if (typeof t !== "number" || t <= 0) return 5000;
	return Math.min(t, 20000);
}

async function executeAction(command: Command): Promise<unknown> {
	const { action, target, value, options } = command;

	// At this point, target/value have been validated for actions that need them.
	// requireTarget/requireValue provide type-safe non-null returns.
	// On Chrome, trusted (CDP) input is dispatched by the background. The WS/HTTP
	// path reaches the content script here; the native-host path is routed in
	// `sendCommandToTab` (background) and never invokes these handlers, so this
	// branch only affects the server/WebSocket path. Relay click/dblclick/
	// rightclick/pressKey/type and return the background's result. On Firefox (no
	// `chrome.debugger`) — or if the background reports CDP input unsupported —
	// `relayCdpInput` returns null and we fall through to the synthetic handlers
	// below (which are also upgraded to emit pointer events and correct key codes).
	if (CDP_INPUT_RELAY_ACTIONS.has(action) && cdpInputAvailable()) {
		const relayed = await relayCdpInput(command);
		if (relayed) return relayed;
	}

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
				waitTimeout(options),
			);
		case "dblclick":
			return handleClick(
				requireTarget(target, action),
				"left",
				2,
				waitTimeout(options),
			);
		case "rightclick":
			return handleClick(
				requireTarget(target, action),
				"right",
				1,
				waitTimeout(options),
			);
		case "hover":
			return handleHover(requireTarget(target, action), waitTimeout(options));
		case "focus":
			return handleFocus(requireTarget(target, action), waitTimeout(options));
		case "blur":
			return handleBlur(requireTarget(target, action), waitTimeout(options));
		case "scrollTo":
			return handleScrollTo(
				requireTarget(target, action),
				waitTimeout(options),
			);
		case "fill":
			return handleFill(
				requireTarget(target, action),
				requireValue(value, action),
				waitTimeout(options),
			);
		case "type":
			return handleType(
				requireTarget(target, action),
				requireValue(value, action),
				waitTimeout(options),
			);
		case "clear":
			return handleClear(requireTarget(target, action), waitTimeout(options));
		case "select":
			return handleSelect(
				requireTarget(target, action),
				requireValue(value, action),
				waitTimeout(options),
			);
		case "check":
			return handleCheck(
				requireTarget(target, action),
				true,
				waitTimeout(options),
			);
		case "uncheck":
			return handleCheck(
				requireTarget(target, action),
				false,
				waitTimeout(options),
			);
		case "pressKey":
			// Target is optional: targetless press goes to the focused element.
			return handlePressKey(
				target,
				requireValue(value, action),
				waitTimeout(options),
			);
		case "selectText":
			return handleSelectText(
				requireTarget(target, action),
				waitTimeout(options),
			);

		// ─── Internal CDP preparation (invoked by the background) ─────
		// These never run on the user-facing path. The background's trusted
		// (CDP) click/key/type dispatchers call them to wait for the element,
		// scroll it into view, and (for keys) focus it — then report the
		// geometry / focus state the CDP dispatch needs.
		case "prepareClick":
			return handlePrepareClick(
				requireTarget(target, action),
				waitTimeout(options),
			);
		case "prepareKeys":
			// Target is optional: targetless press goes to the focused element.
			return handlePrepareKeys(target, waitTimeout(options));

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
			return handleHighlight(
				requireTarget(target, action),
				waitTimeout(options),
			);
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
		case EVALUATE_VIA_CDP:
			// Server-routed CDP evaluation. Forward to the background, which
			// owns the `chrome.debugger` connection and runs
			// `Runtime.evaluate` in the page's main world. Returns the
			// value to the caller; the result is shaped like any other
			// CommandResult data field.
			return handleEvaluateViaCdp(requireValue(value, action));

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
): Promise<RemoteElementInfo> {
	const timeoutMs = timeout ?? 5000;
	// `wait` always waits for the element to appear (the `waitForAppear` gate
	// does not apply) and fails loudly on timeout instead of resolving null.
	const element = await waitForElement(target, timeoutMs, {
		force: true,
		throwOnTimeout: true,
	});
	if (!element) {
		const label = target.selector ?? target.xpath ?? JSON.stringify(target);
		throw new Error(
			`wait: element "${label}" did not appear within ${timeoutMs}ms`,
		);
	}
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

async function handleClick(
	target: TargetSelector,
	button = "left",
	count = 1,
	timeoutMs = 5000,
): Promise<void> {
	const element = await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: true,
	});

	// Scroll into view FIRST (instant, centered) so the synthesized events land
	// on the element's real geometry. Dispatching before scrolling is the wrong
	// order and can miss an off-viewport target.
	element.scrollIntoView({ behavior: "auto", block: "center" });

	const rect = element.getBoundingClientRect();
	const x = rect.left + rect.width / 2;
	const y = rect.top + rect.height / 2;

	const eventInit: MouseEventInit = {
		bubbles: true,
		cancelable: true,
		view: window,
		button: button === "right" ? 2 : 0,
		clientX: x,
		clientY: y,
	};

	const pointerButton = button === "right" ? 2 : 0;
	const pointerInit: PointerEventInit = {
		bubbles: true,
		cancelable: true,
		view: window,
		pointerId: 1,
		pointerType: "mouse",
		button: pointerButton,
		buttons: pointerButton,
		clientX: x,
		clientY: y,
	};
	const makePointer = (type: string) =>
		typeof PointerEvent !== "undefined"
			? new PointerEvent(type, pointerInit)
			: new MouseEvent(type, eventInit);

	// Full event sequence matching real browser order: pointer events are
	// dispatched immediately before/after their mouse counterparts (pointerdown
	// before mousedown, pointerup before mouseup). This is what pages expect and
	// is required for frameworks that listen to pointer events.
	const singleClickSequence = [
		"pointerover",
		"mouseover",
		"mouseenter",
		"pointerdown",
		"mousedown",
		"pointerup",
		"mouseup",
		"click",
	];

	for (let i = 0; i < count; i++) {
		for (const eventType of singleClickSequence) {
			if (eventType.startsWith("pointer")) {
				(element as HTMLElement).dispatchEvent(makePointer(eventType));
			} else {
				(element as HTMLElement).dispatchEvent(
					new MouseEvent(eventType, eventInit),
				);
			}
		}
	}

	// dblclick fires after both click sequences, matching the DOM Level 3 event order
	if (count === 2) {
		(element as HTMLElement).dispatchEvent(
			new MouseEvent("dblclick", eventInit),
		);
	}
}

async function handleHover(
	target: TargetSelector,
	timeoutMs = 5000,
): Promise<void> {
	const element = await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: false,
	});

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

async function handleFocus(
	target: TargetSelector,
	timeoutMs = 5000,
): Promise<void> {
	const element = await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: false,
	});
	(element as HTMLElement).focus();
}

async function handleBlur(
	target: TargetSelector,
	timeoutMs = 5000,
): Promise<void> {
	const element = await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: false,
	});
	(element as HTMLElement).blur();
}

/**
 * Wait until the page scroll position is stable across two consecutive
 * animation frames, so a command issued right after `scrollTo` (e.g. a
 * screenshot) observes the settled position rather than mid-animation.
 * Bounded by a short hard cap so a page with its own scroll animations can't
 * hang the command.
 */
function waitForScrollSettle(maxMs = 500): Promise<void> {
	return new Promise((resolve) => {
		const start = Date.now();
		let lastTop = window.scrollY;
		let lastLeft = window.scrollX;
		let stableFrames = 0;
		const check = () => {
			const top = window.scrollY;
			const left = window.scrollX;
			if (top === lastTop && left === lastLeft) {
				stableFrames++;
			} else {
				stableFrames = 0;
				lastTop = top;
				lastLeft = left;
			}
			if (stableFrames >= 2 || Date.now() - start > maxMs) {
				resolve();
				return;
			}
			requestAnimationFrame(check);
		};
		requestAnimationFrame(check);
	});
}

async function handleScrollTo(
	target: TargetSelector,
	timeoutMs = 5000,
): Promise<void> {
	// Auto-wait for the target to be visible (no enabled requirement).
	const element = await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: false,
	});
	// Instant scroll (no smooth animation) so the following action captures the
	// settled position; then wait for the scroll to actually settle.
	element.scrollIntoView({ behavior: "auto", block: "center" });
	await waitForScrollSettle();
}

async function handleFill(
	target: TargetSelector,
	value: string,
	timeoutMs = 5000,
): Promise<void> {
	const element = (await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: true,
	})) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement;

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

async function handleType(
	target: TargetSelector,
	value: string,
	timeoutMs = 5000,
): Promise<void> {
	const element = (await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: true,
	})) as HTMLInputElement | HTMLTextAreaElement;

	if (
		element instanceof HTMLInputElement ||
		element instanceof HTMLTextAreaElement
	) {
		(element as HTMLInputElement).focus();

		// Type each character
		for (const char of value) {
			// Resolve the physical key code from the descriptor (correct for
			// letters, digits, and symbols) instead of the old hand-built
			// "Key" + char string. Falls back if the char isn't in the table.
			const charCode = (() => {
				try {
					return resolveKey(char).code;
				} catch {
					return `Key${char.toUpperCase()}`;
				}
			})();
			const keyDownEvent = new KeyboardEvent("keydown", {
				bubbles: true,
				key: char,
				code: charCode,
			});
			const keyPressEvent = new KeyboardEvent("keypress", {
				bubbles: true,
				key: char,
				code: charCode,
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
				code: charCode,
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

async function handleClear(
	target: TargetSelector,
	timeoutMs = 5000,
): Promise<void> {
	const element = (await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: true,
	})) as HTMLInputElement | HTMLTextAreaElement;

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

async function handleSelect(
	target: TargetSelector,
	value: string,
	timeoutMs = 5000,
): Promise<void> {
	const element = (await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: true,
	})) as HTMLSelectElement;

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

async function handleCheck(
	target: TargetSelector,
	checked: boolean,
	timeoutMs = 5000,
): Promise<void> {
	const element = (await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: true,
	})) as HTMLInputElement;

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

async function handlePressKey(
	target: TargetSelector | undefined,
	key: string,
	timeoutMs = 5000,
): Promise<void> {
	// Playwright keyboard.press semantics: without a target, the key goes to
	// the currently focused element (e.g. after a `fill`) — falling back to
	// body when nothing has focus. With a target, wait for it and focus it.
	const element = target
		? ((await waitForActionableElement(target, {
				timeoutMs,
				requireEnabled: true,
			})) as HTMLElement)
		: ((document.activeElement as HTMLElement | null) ?? document.body);

	element.focus();

	// Derive the correct `key`/`code`/`windowsVirtualKeyCode` from the descriptor
	// instead of the old hand-built `"Key" + key` string, which produced wrong
	// codes for anything that wasn't a single letter (e.g. "Enter" → "KeyEnter").
	const descriptor = resolveKey(key);
	const keyInit: KeyboardEventInit = {
		bubbles: true,
		key: descriptor.key,
		code: descriptor.code,
		keyCode: descriptor.windowsVirtualKeyCode,
		which: descriptor.windowsVirtualKeyCode,
	};
	const keyDownEvent = new KeyboardEvent("keydown", keyInit);
	const keyUpEvent = new KeyboardEvent("keyup", keyInit);

	element.dispatchEvent(keyDownEvent);
	element.dispatchEvent(keyUpEvent);
}

/**
 * Internal: prepare an element for a trusted (CDP) click.
 *
 * Waits for the element to be actionable (visible + enabled), scrolls it into
 * view (instant, centered), and reports its viewport-center coordinates
 * (post-scroll, CSS pixels) — exactly the coordinates CDP `Input.dispatchMouseEvent`
 * expects. Returns `{ x, y }`.
 */
async function handlePrepareClick(
	target: TargetSelector,
	timeoutMs = 5000,
): Promise<{ x: number; y: number }> {
	const element = await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: true,
	});

	// Scroll into view FIRST (instant, centered) so the reported coordinates
	// reflect the element's real, on-screen geometry.
	element.scrollIntoView({ behavior: "auto", block: "center" });

	const rect = element.getBoundingClientRect();
	const x = rect.left + rect.width / 2;
	const y = rect.top + rect.height / 2;
	return { x, y };
}

/**
 * Internal: prepare an element for a trusted (CDP) key/type.
 *
 * Waits for the element to be actionable, scrolls it into view, focuses it,
 * and confirms the focus actually landed (so the subsequent CDP key events
 * reach the right element). Returns `{ focused }`.
 */
async function handlePrepareKeys(
	target: TargetSelector | undefined,
	timeoutMs = 5000,
): Promise<{ focused: boolean }> {
	// Targetless press (htrcli `press Enter` after a `fill`) follows Playwright
	// keyboard.press semantics: the CDP key events go to whatever currently has
	// focus, so there is nothing to wait for or refocus here.
	if (!target) {
		return { focused: document.activeElement != null };
	}
	const element = (await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: true,
	})) as HTMLElement;

	element.scrollIntoView({ behavior: "auto", block: "center" });
	element.focus();
	return { focused: document.activeElement === element };
}

async function handleSelectText(
	target: TargetSelector,
	timeoutMs = 5000,
): Promise<void> {
	const element = await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: false,
	});

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

	// SECURITY: This uses the `Function` constructor to execute arbitrary code
	// with full page access. It is intended for trusted browser automation and
	// the server enforces authentication before forwarding commands.
	//
	// ISOLATED WORLD: the script runs in the extension's isolated world, so it
	// cannot see page-context JavaScript globals/variables. Use `debuggerEval`
	// for page-context evaluation.
	//
	// Compilation uses two deterministic modes chosen at compile time (NOT a
	// runtime fallback): first as a single expression (preserving the
	// `document.title` style usage); if that is a SyntaxError, as a function
	// body where the caller supplies an explicit `return`. Both are executed
	// inside an async function so `await` works, and the resolved value is
	// returned. Runtime errors from the script propagate unchanged.
	let compiled: () => unknown;
	try {
		// Mode 1: a single expression.
		compiled = new Function(`return ( ${script} );`) as () => unknown;
	} catch (err) {
		if (!(err instanceof SyntaxError)) throw err;
		// Mode 2: a statement list / function body with an explicit `return`.
		try {
			compiled = new Function(
				`return (async () => { ${script} })();`,
			) as () => unknown;
		} catch (err2) {
			// Genuinely invalid in both modes. Normalize the message so callers
			// (and tests) get a consistent "SyntaxError" indicator across
			// runtimes — V8 prefixes `.message` with "SyntaxError:" but
			// JavaScriptCore does not.
			const msg = err2 instanceof Error ? err2.message : String(err2);
			throw new Error(
				msg.includes("SyntaxError") ? msg : `SyntaxError: ${msg}`,
			);
		}
	}

	return (async () => compiled())();
}

/**
 * Forward a `Runtime.evaluate` request to the background script. The
 * background owns the `chrome.debugger` connection; the content script acts
 * as a passthrough. On Firefox (no `chrome.debugger`), the background will
 * reply with an explicit error and we re-throw it so callers see a clear
 * message instead of a silent fallback to the isolated-world path.
 */
async function handleEvaluateViaCdp(expression: string): Promise<unknown> {
	if (!expression) throw new Error("evaluateViaCdp: expression is required");
	if (typeof chrome.runtime?.sendMessage !== "function") {
		throw new Error("evaluateViaCdp: chrome.runtime.sendMessage unavailable");
	}
	const tabId = await new Promise<number>((resolve) => {
		chrome.runtime.sendMessage(
			{ type: "GET_CURRENT_TAB_ID" },
			(response: { tabId: number } | undefined) =>
				resolve(response?.tabId ?? 0),
		);
	});
	if (!tabId)
		throw new Error("evaluateViaCdp: could not resolve current tab id");
	return new Promise<unknown>((resolve, reject) => {
		chrome.runtime.sendMessage(
			{ type: "CDP_EVAL", tabId, expression },
			(resp: { ok: boolean; data?: unknown; error?: string } | undefined) => {
				if (!resp) {
					reject(new Error("evaluateViaCdp: no response from background"));
					return;
				}
				if (!resp.ok) {
					reject(new Error(resp.error ?? "evaluateViaCdp: background error"));
					return;
				}
				resolve(resp.data);
			},
		);
	});
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
			if (p.jwt) headers.Authorization = `Bearer ${p.jwt}`;
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
		bin += String.fromCharCode(
			...bytes.subarray(i, Math.min(i + 8192, bytes.length)),
		);
	}
	return { status, contentType, base64: btoa(bin) };
}

// Runs fetch in the PAGE's JS context (MAIN world) via <script> injection,
// so the request goes out with Origin: https://www.aia.com.my instead of the
// extension origin. Requires the page to have no CSP blocking inline scripts.
const FETCH_IN_PAGE_ALLOWED_HOSTS = new Set([
	"api.aia.com.my",
	"www.aia.com.my",
]);

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
				if (parsed.jwt) defaultHeaders.Authorization = `Bearer ${parsed.jwt}`;
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

async function handleHighlight(
	target: TargetSelector,
	timeoutMs = 5000,
): Promise<RemoteElementInfo | null> {
	// Auto-wait for the target to be visible (no enabled requirement) before
	// highlighting it.
	const element = await waitForActionableElement(target, {
		timeoutMs,
		requireEnabled: false,
	});

	// Dispatch custom event for the highlighter to pick up
	window.dispatchEvent(
		new CustomEvent("htrncontrol:highlight", {
			detail: { element },
		}),
	);

	return getElementInfo(element);
}

function handleUnhighlight(): void {
	window.dispatchEvent(new CustomEvent("htrncontrol:unhighlight"));
}

function safeHistoryLength(): number {
	try {
		const length = window.history?.length;
		return typeof length === "number" ? length : 0;
	} catch (err) {
		// happy-dom's `window.history` getter can throw a TypeError when the
		// browser frame isn't fully initialized. Real pages don't throw here,
		// so an unexpected exception is interesting — log it so a future bug
		// isn't silently swallowed.
		if (err instanceof TypeError) return 0;
		console.warn("[HTR NControl] safeHistoryLength: unexpected error:", err);
		return 0;
	}
}

// ─── Page Info ─────────────────────────────────────────────────────

function getPageInfo(): PageInfo {
	return {
		url: window.location.href,
		title: document.title,
		domain: window.location.hostname,
		readyState: document.readyState,
		scrollX: window.scrollX,
		scrollY: window.scrollY,
		viewportWidth: window.innerWidth,
		viewportHeight: window.innerHeight,
		documentHeight: document.documentElement.scrollHeight,
		documentWidth: document.documentElement.scrollWidth,
		// happy-dom's `window.history` getter can throw a TypeError when the
		// browser frame isn't fully initialized. Guard so the rest of the
		// PageInfo still comes through (the goBack/goForward pre-check only
		// uses this as a hint; a missing value is fine — the runtime race is
		// authoritative).
		historyLength: safeHistoryLength(),
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
