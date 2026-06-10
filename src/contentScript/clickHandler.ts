/**
 * Click Handler
 * Detects and processes click events on interactive elements
 */

import type { ClickEventMessage, ElementInfo } from "../types/recording";
import { generateSelector } from "./selectorGenerator";

// Elements that we want to track clicks on
const INTERACTIVE_ELEMENTS = [
	"a",
	"button",
	"input",
	"select",
	"textarea",
	'[role="button"]',
	'[role="link"]',
	'[role="checkbox"]',
	'[role="radio"]',
	'[role="tab"]',
	'[role="menuitem"]',
	'[role="option"]',
	"[onclick]",
	"[tabindex]",
];

// Input types that we track clicks on (not text inputs - those are handled by input handler)
const CLICKABLE_INPUT_TYPES = [
	"submit",
	"button",
	"reset",
	"checkbox",
	"radio",
	"file",
	"image",
];

// Recording start timestamp (set when recording begins)
let recordingStartTime: number | null = null;

// Click handler callback
let clickCallback: ((message: ClickEventMessage) => void) | null = null;

/**
 * Check if an element is interactive and should be tracked
 */
function isInteractiveElement(element: Element): boolean {
	// Check if it matches any interactive selector
	for (const selector of INTERACTIVE_ELEMENTS) {
		if (element.matches(selector)) {
			// For inputs, only track clicks on non-text inputs
			if (element instanceof HTMLInputElement) {
				return CLICKABLE_INPUT_TYPES.includes(element.type.toLowerCase());
			}
			return true;
		}
	}

	// Check if parent is a link or button (for nested elements like icons)
	const parent = element.closest('a, button, [role="button"]');
	if (parent) {
		return true;
	}

	return false;
}

/**
 * Find the most relevant interactive element for a click
 * (handles clicks on nested elements like icons inside buttons)
 */
function findInteractiveElement(target: Element): Element | null {
	// First check if the target itself is interactive
	if (isInteractiveElement(target)) {
		// For inputs, return as-is
		if (target instanceof HTMLInputElement) {
			return target;
		}
		// For links/buttons, return the actual interactive element
		const interactive = target.closest(
			'a, button, [role="button"], [role="link"]',
		);
		return interactive || target;
	}

	// Check ancestors for interactive elements
	const interactive = target.closest(INTERACTIVE_ELEMENTS.join(", "));
	if (interactive) {
		return interactive;
	}

	return null;
}

/**
 * Get meaningful text from an element
 * Checks multiple sources: direct text, aria-label, title, value, alt, placeholder
 * Also looks at child elements for text content
 */
function getElementText(element: Element): string {
	// 1. Check aria-label first (most explicit)
	const ariaLabel = element.getAttribute("aria-label");
	if (ariaLabel?.trim()) {
		return ariaLabel.trim().slice(0, 100);
	}

	// 2. Check title attribute
	const title = element.getAttribute("title");
	if (title?.trim()) {
		return title.trim().slice(0, 100);
	}

	// 3. For inputs, check value and placeholder
	if (element instanceof HTMLInputElement) {
		if (element.type === "submit" || element.type === "button") {
			if (element.value?.trim()) {
				return element.value.trim().slice(0, 100);
			}
		}
		if (element.placeholder?.trim()) {
			return element.placeholder.trim().slice(0, 100);
		}
	}

	// 4. For images, check alt text
	if (element instanceof HTMLImageElement && element.alt?.trim()) {
		return element.alt.trim().slice(0, 100);
	}

	// 5. Get direct text content (excluding deeply nested text)
	const directText = getDirectTextContent(element);
	if (directText) {
		return directText.slice(0, 100);
	}

	// 6. Look for text in immediate children (buttons with spans, links with text nodes)
	const childText = getChildTextContent(element);
	if (childText) {
		return childText.slice(0, 100);
	}

	// 7. Fall back to full textContent as last resort
	const fullText = element.textContent?.trim();
	if (fullText) {
		return fullText.slice(0, 100);
	}

	return "";
}

/**
 * Get direct text content of an element (text nodes that are direct children)
 */
function getDirectTextContent(element: Element): string {
	let text = "";
	for (const node of element.childNodes) {
		if (node.nodeType === Node.TEXT_NODE) {
			const nodeText = node.textContent?.trim();
			if (nodeText) {
				text += (text ? " " : "") + nodeText;
			}
		}
	}
	return text.trim();
}

/**
 * Get text content from child elements (for elements like <button><span>Click me</span></button>)
 * Prioritizes visible text elements and skips hidden/icon elements
 */
function getChildTextContent(element: Element): string {
	const textParts: string[] = [];

	// Look for common text-containing children
	const textElements = element.querySelectorAll(
		'span, strong, em, b, i, label, p, div:not([class*="icon"]):not([class*="svg"])',
	);

	for (const child of textElements) {
		// Skip elements that are likely icons or hidden
		const classList = child.className?.toLowerCase() || "";
		if (
			classList.includes("icon") ||
			classList.includes("svg") ||
			classList.includes("sr-only") ||
			classList.includes("visually-hidden")
		) {
			continue;
		}

		// Skip if element is hidden
		const style = window.getComputedStyle(child);
		if (style.display === "none" || style.visibility === "hidden") {
			continue;
		}

		const text = child.textContent?.trim();
		if (text && text.length > 0 && text.length < 200) {
			textParts.push(text);
		}
	}

	// Deduplicate and join
	const uniqueText = [...new Set(textParts)].join(" ").trim();
	return uniqueText;
}

/**
 * Get element information for the recording
 */
function getElementInfo(element: Element): ElementInfo {
	const tag = element.tagName.toLowerCase();
	const text = getElementText(element);
	const selector = generateSelector(element);

	const info: ElementInfo = {
		tag,
		text,
		selector,
	};

	// Add input-specific info
	if (element instanceof HTMLInputElement) {
		info.type = element.type;
		info.name = element.name || undefined;
	}

	// Add ID if present
	if (element.id) {
		info.id = element.id;
	}

	// Add class names (first few)
	if (element.classList.length > 0) {
		info.className = Array.from(element.classList).slice(0, 5).join(" ");
	}

	// Add aria-label if present (also store separately for reference)
	const ariaLabel = element.getAttribute("aria-label");
	if (ariaLabel) {
		info.ariaLabel = ariaLabel;
	}

	return info;
}

/**
 * Handle a click event
 */
function handleClick(event: MouseEvent): void {
	// Only process if recording is active
	if (recordingStartTime === null || clickCallback === null) {
		return;
	}

	const target = event.target as Element;
	if (!target) return;

	// Find the relevant interactive element
	const interactiveElement = findInteractiveElement(target);
	if (!interactiveElement) return;

	// Don't track clicks on the extension's own UI
	if (interactiveElement.closest("[data-how-to-recorder-ignore]")) {
		return;
	}

	// Get element info
	const elementInfo = getElementInfo(interactiveElement);

	// Calculate timestamp relative to recording start
	const timestamp = Date.now() - recordingStartTime;

	// Create the message
	const message: ClickEventMessage = {
		type: "CLICK_EVENT",
		element: elementInfo,
		url: window.location.href,
		timestamp,
	};

	// Send to background script
	clickCallback(message);
}

/**
 * Start listening for click events
 */
export function startClickTracking(
	startTime: number,
	callback: (message: ClickEventMessage) => void,
): void {
	recordingStartTime = startTime;
	clickCallback = callback;

	// Use capture phase to catch events before they might be stopped
	document.addEventListener("click", handleClick, { capture: true });
}

/**
 * Stop listening for click events
 */
export function stopClickTracking(): void {
	document.removeEventListener("click", handleClick, { capture: true });
	recordingStartTime = null;
	clickCallback = null;
}

/**
 * Check if click tracking is active
 */
export function isClickTrackingActive(): boolean {
	return recordingStartTime !== null;
}
