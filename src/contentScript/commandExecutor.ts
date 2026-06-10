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

		// ─── Highlight ────────────────────────────────────────────────
		case "highlight":
			return handleHighlight(requireTarget(target, action));
		case "unhighlight":
			return handleUnhighlight();

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
