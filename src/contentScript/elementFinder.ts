/**
 * Element Finder
 * Finds DOM elements using various strategies: CSS selectors, XPath, text, attributes, etc.
 * Used by the command executor to locate elements for remote control actions.
 */

import type {
	RemoteElementInfo,
	TargetSelector,
	TextMatchMode,
} from "../types/commands";
import { generateXPath } from "./xpathGenerator";

// ─── Main Find Functions ──────────────────────────────────────────

/**
 * Find a single element matching the target selector.
 * Returns the first match or null.
 */
export function findElement(target: TargetSelector): Element | null {
	const elements = findAllElementsRaw(target);
	if (elements.length === 0) return null;

	const index = target.index ?? 0;
	return elements[index] ?? null;
}

/**
 * Find all elements matching the target selector.
 */
export function findAllElements(target: TargetSelector): Element[] {
	return findAllElementsRaw(target);
}

/**
 * Find element(s) and return detailed info.
 */
export function findElementInfo(target: TargetSelector): RemoteElementInfo[] {
	const elements = findAllElementsRaw(target);
	return elements.map((el) => getElementInfo(el));
}

/**
 * Wait for an element to appear in the DOM.
 * Uses MutationObserver for efficient detection.
 */
export function waitForElement(
	target: TargetSelector,
	timeoutMs = 5000,
): Promise<Element | null> {
	// Check if already present
	const existing = findElement(target);
	if (existing) return Promise.resolve(existing);

	if (!target.waitForAppear) return Promise.resolve(null);

	return new Promise((resolve) => {
		let resolved = false;
		const timer = setTimeout(() => {
			if (!resolved) {
				obs.disconnect();
				resolve(null);
			}
		}, timeoutMs);

		const obs = new MutationObserver(() => {
			const el = findElement(target);
			if (el && !resolved) {
				resolved = true;
				clearTimeout(timer);
				obs.disconnect();
				resolve(el);
			}
		});

		obs.observe(document.documentElement, {
			childList: true,
			subtree: true,
		});
	});
}

// ─── Raw Finder (internal) ────────────────────────────────────────

function findAllElementsRaw(target: TargetSelector): Element[] {
	// Try strategies in priority order
	let elements: Element[] = [];

	// 1. CSS selector
	if (target.selector) {
		elements = querySelectorAllSafe(target.selector);
		return applyFilters(elements, target);
	}

	// 2. XPath
	if (target.xpath) {
		elements = queryXPath(target.xpath);
		return applyFilters(elements, target);
	}

	// 3. ID
	if (target.id) {
		const el = document.getElementById(target.id);
		if (el) elements = [el];
		return applyFilters(elements, target);
	}

	// 4. Name attribute
	if (target.name) {
		elements = querySelectorAllSafe(`[name="${CSS.escape(target.name)}"]`);
		if (elements.length > 0) return applyFilters(elements, target);
		// Fallback: search all elements with name attribute
		elements = Array.from(document.querySelectorAll(`[name="${target.name}"]`));
		return applyFilters(elements, target);
	}

	// 5. Role
	if (target.role) {
		elements = querySelectorAllSafe(`[role="${target.role}"]`);
		if (elements.length > 0) return applyFilters(elements, target);
	}

	// 6. Label (find form control by associated label)
	if (target.label) {
		elements = findByLabel(target.label);
		return applyFilters(elements, target);
	}

	// 7. Placeholder
	if (target.placeholder) {
		elements = querySelectorAllSafe(
			`[placeholder="${CSS.escape(target.placeholder)}"]`,
		);
		if (elements.length > 0) return applyFilters(elements, target);
	}

	// 8. Text content
	if (target.text) {
		elements = findByText(
			target.text,
			target.tag,
			target.textMatch,
			target.caseSensitive,
		);
		return applyFilters(elements, target);
	}

	// 9. Tag + type (generic)
	if (target.tag) {
		let selector = target.tag;
		if (target.type) {
			selector += `[type="${target.type}"]`;
		}
		elements = querySelectorAllSafe(selector);
		return applyFilters(elements, target);
	}

	return [];
}

// ─── Strategy Implementations ──────────────────────────────────────

/**
 * Safe querySelectorAll that catches invalid selectors
 */
function querySelectorAllSafe(selector: string): Element[] {
	try {
		return Array.from(document.querySelectorAll(selector));
	} catch {
		return [];
	}
}

/**
 * Execute an XPath expression and return matching elements
 */
function queryXPath(xpath: string): Element[] {
	try {
		const result = document.evaluate(
			xpath,
			document,
			null,
			XPathResult.ORDERED_NODE_SNAPSHOT_TYPE,
			null,
		);
		const elements: Element[] = [];
		for (let i = 0; i < result.snapshotLength; i++) {
			const node = result.snapshotItem(i);
			if (node instanceof Element) {
				elements.push(node);
			}
		}
		return elements;
	} catch {
		return [];
	}
}

/**
 * Find form element by associated label text
 */
function findByLabel(labelText: string): Element[] {
	// Find label elements matching the text
	const labels = Array.from(document.querySelectorAll("label"));
	for (const label of labels) {
		// Check label text content
		if (getTextContent(label).trim() === labelText.trim()) {
			// Check for "for" attribute
			if (label.htmlFor) {
				const control = document.getElementById(label.htmlFor);
				if (control) return [control];
			}
			// Check for nested control
			const nested = label.querySelector("input, select, textarea");
			if (nested) return [nested];
		}
	}

	// Fallback: look for aria-labelledby
	const allControls = document.querySelectorAll("input, select, textarea");
	for (const control of allControls) {
		const labelledBy = control.getAttribute("aria-labelledby");
		if (labelledBy) {
			const labelEl = document.getElementById(labelledBy);
			if (labelEl && getTextContent(labelEl).trim() === labelText.trim()) {
				return [control];
			}
		}

		// Check aria-label
		const ariaLabel = control.getAttribute("aria-label");
		if (ariaLabel && ariaLabel.trim() === labelText.trim()) {
			return [control];
		}
	}

	return [];
}

/**
 * Find elements by text content
 */
function findByText(
	text: string,
	tag?: string,
	matchMode: TextMatchMode = "contains",
	caseSensitive = false,
): Element[] {
	const results: Element[] = [];
	const selector = tag || "*";
	const elements = document.querySelectorAll(selector);

	for (const el of elements) {
		const textContent = getTextContent(el);
		if (matchText(textContent, text, matchMode, caseSensitive)) {
			results.push(el);
		}
	}

	return results;
}

/**
 * Match text content against a pattern
 */
function matchText(
	content: string,
	pattern: string,
	mode: TextMatchMode,
	caseSensitive: boolean,
): boolean {
	const a = caseSensitive ? content : content.toLowerCase();
	const b = caseSensitive ? pattern : pattern.toLowerCase();

	switch (mode) {
		case "exact":
			return a === b;
		case "contains":
			return a.includes(b);
		case "startsWith":
			return a.startsWith(b);
		case "endsWith":
			return a.endsWith(b);
		case "regex":
			try {
				const flags = caseSensitive ? "" : "i";
				return new RegExp(pattern, flags).test(content);
			} catch {
				return false;
			}
		default:
			return a.includes(b);
	}
}

// ─── Filters ──────────────────────────────────────────────────────

function applyFilters(elements: Element[], target: TargetSelector): Element[] {
	let result = elements;

	if (target.visible) {
		result = result.filter(isVisible);
	}

	if (target.enabled) {
		result = result.filter(isEnabled);
	}

	return result;
}

/**
 * Check if an element is visible in the viewport
 */
function isVisible(element: Element): boolean {
	if (!element.ownerDocument) return false;

	const style = window.getComputedStyle(element);
	if (
		style.display === "none" ||
		style.visibility === "hidden" ||
		style.opacity === "0"
	) {
		return false;
	}

	const rect = element.getBoundingClientRect();
	if (rect.width === 0 || rect.height === 0) return false;

	// Check if at least partially in viewport
	const viewportWidth =
		window.innerWidth || document.documentElement.clientWidth;
	const viewportHeight =
		window.innerHeight || document.documentElement.clientHeight;

	return (
		rect.bottom > 0 &&
		rect.right > 0 &&
		rect.top < viewportHeight &&
		rect.left < viewportWidth
	);
}

/**
 * Check if an element is enabled (not disabled)
 */
function isEnabled(element: Element): boolean {
	if (element instanceof HTMLInputElement) return !element.disabled;
	if (element instanceof HTMLSelectElement) return !element.disabled;
	if (element instanceof HTMLTextAreaElement) return !element.disabled;
	if (element instanceof HTMLButtonElement) return !element.disabled;
	if (element instanceof HTMLFieldSetElement) return !element.disabled;

	const ariaDisabled = element.getAttribute("aria-disabled");
	if (ariaDisabled === "true") return false;

	return true;
}

// ─── Element Info Extraction ──────────────────────────────────────

/**
 * Get comprehensive info about an element
 */
export function getElementInfo(element: Element): RemoteElementInfo {
	const rect = element.getBoundingClientRect();
	const text = getTextContent(element);

	// Collect all attributes
	const attributes: Record<string, string> = {};
	for (const attr of Array.from(element.attributes)) {
		attributes[attr.name] = attr.value;
	}

	return {
		tag: element.tagName.toLowerCase(),
		text: text.substring(0, 500), // Limit text length
		selector: generateCSSSelector(element),
		xpath: generateXPath(element),
		type: getAttributeSafe(element, "type"),
		name: getAttributeSafe(element, "name"),
		id: element.id || undefined,
		className: element.className ? String(element.className).trim() : undefined,
		ariaLabel: getAttributeSafe(element, "aria-label"),
		value:
			element instanceof HTMLInputElement ||
			element instanceof HTMLTextAreaElement
				? element.value
				: undefined,
		visible: isVisible(element),
		enabled: isEnabled(element),
		boundingBox: {
			x: rect.x,
			y: rect.y,
			width: rect.width,
			height: rect.height,
			top: rect.top,
			bottom: rect.bottom,
			left: rect.left,
			right: rect.right,
		},
		attributes,
	};
}

// ─── Utility Functions ────────────────────────────────────────────

/**
 * Get text content from an element, including special attributes
 */
function getTextContent(element: Element): string {
	// For inputs, check value first
	if (element instanceof HTMLInputElement) {
		if (element.type === "button" || element.type === "submit") {
			return element.value || element.textContent || "";
		}
		return element.value || "";
	}
	if (element instanceof HTMLTextAreaElement) {
		return element.value || "";
	}
	if (element instanceof HTMLSelectElement) {
		const selected = element.options[element.selectedIndex];
		return selected?.textContent || "";
	}

	// For other elements, try multiple text sources
	const sources = [
		element.textContent,
		getAttributeSafe(element, "aria-label"),
		getAttributeSafe(element, "title"),
		getAttributeSafe(element, "alt"),
	];

	return sources.find((s) => s?.trim()) || "";
}

/**
 * Safely get an attribute value
 */
function getAttributeSafe(element: Element, name: string): string | undefined {
	return element.getAttribute(name) || undefined;
}

/**
 * Generate a CSS selector for an element (simplified version)
 * For a more robust selector, use selectorGenerator.ts
 */
function generateCSSSelector(element: Element): string {
	// ID-based
	if (element.id) {
		return `#${CSS.escape(element.id)}`;
	}

	// Build path from element to root
	const parts: string[] = [];
	let current: Element | null = element;

	while (current && current !== document.documentElement) {
		let selector = current.tagName.toLowerCase();

		if (current.id) {
			parts.unshift(`#${CSS.escape(current.id)}`);
			break;
		}

		// Add classes
		if (current.className && typeof current.className === "string") {
			const classes = current.className
				.trim()
				.split(/\s+/)
				.filter((c) => c && !c.startsWith("__"))
				.map((c) => `.${CSS.escape(c)}`)
				.join("");
			selector += classes;
		}

		// Add nth-child if needed
		if (current.parentElement) {
			const currentTag = current.tagName;
			const siblings = Array.from(current.parentElement.children);
			const sameTag = siblings.filter((s) => s.tagName === currentTag);
			if (sameTag.length > 1) {
				const index = sameTag.indexOf(current) + 1;
				selector += `:nth-of-type(${index})`;
			}
		}

		parts.unshift(selector);
		current = current.parentElement;
	}

	return parts.join(" > ");
}
