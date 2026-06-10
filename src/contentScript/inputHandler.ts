/**
 * Input Handler
 * Monitors form field inputs and detects value changes
 * Handles debouncing to avoid recording every keystroke
 */

import type { ElementInfo, InputEventMessage } from "../types/recording";
import { getRecordableValue } from "../utils/sensitiveFields";
import { generateSelector } from "./selectorGenerator";

// Recording start timestamp
let recordingStartTime: number | null = null;

// Input handler callback
let inputCallback: ((message: InputEventMessage) => void) | null = null;

// Debounce timeout per element (to avoid recording every keystroke)
const INPUT_DEBOUNCE_MS = 1000;

// Map of element to debounce timeout
const debounceTimers = new Map<Element, ReturnType<typeof setTimeout>>();

// Map of element to last recorded value
const lastRecordedValues = new Map<Element, string>();

/**
 * Get the label text for a form element
 * Checks: associated label, aria-label, aria-labelledby, placeholder, title
 */
function getFieldLabel(
	element: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement,
): string {
	// 1. Check aria-label
	const ariaLabel = element.getAttribute("aria-label");
	if (ariaLabel?.trim()) {
		return ariaLabel.trim();
	}

	// 2. Check aria-labelledby
	const labelledBy = element.getAttribute("aria-labelledby");
	if (labelledBy) {
		const labelElement = document.getElementById(labelledBy);
		if (labelElement?.textContent?.trim()) {
			return labelElement.textContent.trim();
		}
	}

	// 3. Check for explicit label via for attribute
	if (element.id) {
		const label = document.querySelector(`label[for="${element.id}"]`);
		if (label?.textContent?.trim()) {
			return label.textContent.trim();
		}
	}

	// 4. Check for implicit label (input inside label)
	const parentLabel = element.closest("label");
	if (parentLabel) {
		// Get label text excluding the input's own text
		const labelClone = parentLabel.cloneNode(true) as HTMLElement;
		const inputs = labelClone.querySelectorAll("input, textarea, select");
		for (const input of inputs) {
			input.remove();
		}
		const labelText = labelClone.textContent?.trim();
		if (labelText) {
			return labelText;
		}
	}

	// 5. Check placeholder
	if ("placeholder" in element && element.placeholder?.trim()) {
		return element.placeholder.trim();
	}

	// 6. Check title attribute
	const title = element.getAttribute("title");
	if (title?.trim()) {
		return title.trim();
	}

	// 7. Check name attribute as fallback (often descriptive)
	if (element.name) {
		// Convert name like "user_email" or "userEmail" to readable text
		return element.name
			.replace(/([A-Z])/g, " $1")
			.replace(/[_-]/g, " ")
			.trim();
	}

	return "";
}

/**
 * Get element information for the recording
 */
function getElementInfo(
	element: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement,
): ElementInfo {
	const tag = element.tagName.toLowerCase();
	const selector = generateSelector(element);
	const text = getFieldLabel(element).slice(0, 100);

	const info: ElementInfo = {
		tag,
		text,
		selector,
	};

	// Add input-specific info
	if (element instanceof HTMLInputElement) {
		info.type = element.type;
	}

	// Add name attribute
	if (element.name) {
		info.name = element.name;
	}

	// Add ID if present
	if (element.id) {
		info.id = element.id;
	}

	// Add aria-label if present
	const ariaLabel = element.getAttribute("aria-label");
	if (ariaLabel) {
		info.ariaLabel = ariaLabel;
	}

	return info;
}

/**
 * Record an input change
 */
function recordInputChange(
	element: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement,
): void {
	if (recordingStartTime === null || inputCallback === null) {
		return;
	}

	// Get the value (masked if sensitive)
	const { value, isSensitive } =
		element instanceof HTMLSelectElement
			? { value: element.value, isSensitive: false }
			: getRecordableValue(element);

	// Skip if value hasn't changed from last recording
	const lastValue = lastRecordedValues.get(element);
	if (lastValue === value) {
		return;
	}

	// Update last recorded value
	lastRecordedValues.set(element, value);

	// Get element info
	const elementInfo = getElementInfo(element);

	// Calculate timestamp relative to recording start
	const timestamp = Date.now() - recordingStartTime;

	// Create the message
	const message: InputEventMessage = {
		type: "INPUT_EVENT",
		element: elementInfo,
		value,
		isSensitive,
		url: window.location.href,
		timestamp,
	};

	console.log(
		"[How-To Recorder] Recording input:",
		elementInfo.selector,
		"value:",
		isSensitive ? "(sensitive)" : value,
	);

	// Send to background script
	inputCallback(message);
}

/**
 * Handle input/change events with debouncing
 */
function handleInputEvent(event: Event): void {
	if (recordingStartTime === null) {
		return;
	}

	const target = event.target as Element;
	if (!target) return;

	// Only handle input, textarea, and select elements
	if (
		!(target instanceof HTMLInputElement) &&
		!(target instanceof HTMLTextAreaElement) &&
		!(target instanceof HTMLSelectElement)
	) {
		return;
	}

	// Don't track extension's own UI
	if (target.closest("[data-how-to-recorder-ignore]")) {
		return;
	}

	// Skip hidden fields and buttons
	if (target instanceof HTMLInputElement) {
		const type = target.type.toLowerCase();
		if (["hidden", "submit", "button", "reset", "image"].includes(type)) {
			return;
		}
	}

	console.log(
		"[How-To Recorder] Input event detected on:",
		target.tagName,
		target instanceof HTMLInputElement ? target.type : "",
	);

	// Clear existing debounce timer for this element
	const existingTimer = debounceTimers.get(target);
	if (existingTimer) {
		clearTimeout(existingTimer);
	}

	// For select elements and checkboxes/radios, record immediately
	if (
		target instanceof HTMLSelectElement ||
		(target instanceof HTMLInputElement &&
			["checkbox", "radio"].includes(target.type.toLowerCase()))
	) {
		recordInputChange(target);
		return;
	}

	// For text inputs, debounce
	const timer = setTimeout(() => {
		recordInputChange(target);
		debounceTimers.delete(target);
	}, INPUT_DEBOUNCE_MS);

	debounceTimers.set(target, timer);
}

/**
 * Handle blur events (record immediately when focus leaves)
 */
function handleBlurEvent(event: Event): void {
	if (recordingStartTime === null) {
		return;
	}

	const target = event.target as Element;
	if (!target) return;

	// Only handle input and textarea elements
	if (
		!(target instanceof HTMLInputElement) &&
		!(target instanceof HTMLTextAreaElement)
	) {
		return;
	}

	// Clear any pending debounce timer
	const existingTimer = debounceTimers.get(target);
	if (existingTimer) {
		clearTimeout(existingTimer);
		debounceTimers.delete(target);
	}

	// Record the current value
	recordInputChange(target);
}

/**
 * Start listening for input events
 */
export function startInputTracking(
	startTime: number,
	callback: (message: InputEventMessage) => void,
): void {
	recordingStartTime = startTime;
	inputCallback = callback;

	// Clear any existing state
	debounceTimers.clear();
	lastRecordedValues.clear();

	// Listen for input events (fires on every change for text inputs)
	document.addEventListener("input", handleInputEvent, { capture: true });

	// Listen for change events (fires on value commit for selects, checkboxes, etc.)
	document.addEventListener("change", handleInputEvent, { capture: true });

	// Listen for blur events (to capture final value when focus leaves)
	document.addEventListener("blur", handleBlurEvent, { capture: true });
}

/**
 * Stop listening for input events
 */
export function stopInputTracking(): void {
	document.removeEventListener("input", handleInputEvent, { capture: true });
	document.removeEventListener("change", handleInputEvent, { capture: true });
	document.removeEventListener("blur", handleBlurEvent, { capture: true });

	// Clear all pending timers
	for (const timer of debounceTimers.values()) {
		clearTimeout(timer);
	}
	debounceTimers.clear();
	lastRecordedValues.clear();

	recordingStartTime = null;
	inputCallback = null;
}

/**
 * Force record all pending inputs (call before stopping)
 */
export function flushPendingInputs(): void {
	// Clear all timers and record current values
	for (const [element, timer] of debounceTimers.entries()) {
		clearTimeout(timer);
		if (
			element instanceof HTMLInputElement ||
			element instanceof HTMLTextAreaElement
		) {
			recordInputChange(element);
		}
	}
	debounceTimers.clear();
}

/**
 * Check if input tracking is active
 */
export function isInputTrackingActive(): boolean {
	return recordingStartTime !== null;
}
