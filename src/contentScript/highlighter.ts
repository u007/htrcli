/**
 * Element Highlighter
 * Adds a visual outline/highlight to elements before screenshot capture
 */

// Highlight overlay element
let highlightOverlay: HTMLDivElement | null = null;

// CSS for the highlight
const HIGHLIGHT_STYLES = {
	position: "fixed",
	pointerEvents: "none",
	zIndex: "2147483647", // Max z-index
	border: "3px solid #ef4444",
	borderRadius: "4px",
	backgroundColor: "rgba(239, 68, 68, 0.1)",
	boxShadow:
		"0 0 0 4px rgba(239, 68, 68, 0.3), 0 0 20px rgba(239, 68, 68, 0.4)",
	transition: "all 0.15s ease-out",
} as const;

/**
 * Create or get the highlight overlay element
 */
function getHighlightOverlay(): HTMLDivElement {
	if (highlightOverlay && document.body.contains(highlightOverlay)) {
		return highlightOverlay;
	}

	highlightOverlay = document.createElement("div");
	highlightOverlay.id = "how-to-recorder-highlight";
	highlightOverlay.setAttribute("data-how-to-recorder-ignore", "true");

	Object.assign(highlightOverlay.style, HIGHLIGHT_STYLES);
	highlightOverlay.style.display = "none";

	document.body.appendChild(highlightOverlay);
	return highlightOverlay;
}

/**
 * Position the highlight overlay over an element
 */
function positionHighlight(element: Element): void {
	const overlay = getHighlightOverlay();
	const rect = element.getBoundingClientRect();

	// Add some padding around the element
	const padding = 4;

	overlay.style.left = `${rect.left - padding}px`;
	overlay.style.top = `${rect.top - padding}px`;
	overlay.style.width = `${rect.width + padding * 2}px`;
	overlay.style.height = `${rect.height + padding * 2}px`;
	overlay.style.display = "block";
}

/**
 * Show highlight on an element
 */
export function showHighlight(element: Element): void {
	positionHighlight(element);
}

/**
 * Hide the highlight overlay
 */
export function hideHighlight(): void {
	if (highlightOverlay) {
		highlightOverlay.style.display = "none";
	}
}

/**
 * Remove the highlight overlay from DOM
 */
export function removeHighlight(): void {
	if (highlightOverlay?.parentNode) {
		highlightOverlay.parentNode.removeChild(highlightOverlay);
		highlightOverlay = null;
	}
}

/**
 * Highlight an element temporarily (for screenshot capture)
 * Returns a promise that resolves after the highlight is visible
 */
export function highlightForScreenshot(
	element: Element,
	duration: number = 100,
): Promise<void> {
	return new Promise((resolve) => {
		showHighlight(element);
		// Small delay to ensure the highlight is rendered before screenshot
		setTimeout(resolve, duration);
	});
}

/**
 * Get the selector for an element that was highlighted
 * Useful for re-highlighting after page changes
 */
export function getHighlightedElementSelector(element: Element): string | null {
	if (element.id) {
		return `#${CSS.escape(element.id)}`;
	}

	// Try to create a unique selector
	const tag = element.tagName.toLowerCase();
	const classes = Array.from(element.classList)
		.slice(0, 3)
		.map((c) => `.${CSS.escape(c)}`)
		.join("");

	if (classes) {
		const selector = `${tag}${classes}`;
		if (document.querySelectorAll(selector).length === 1) {
			return selector;
		}
	}

	return null;
}
