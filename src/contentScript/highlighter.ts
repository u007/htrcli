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
	highlightOverlay.id = "htrncontrol-highlight";
	highlightOverlay.setAttribute("data-htrncontrol-ignore", "true");

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

// ─── Annotation Overlay (absolute-positioned, document-relative) ─────
// Unlike showHighlight (which is position:fixed for a live recording
// overlay), these markers are position:absolute so they scroll with the
// page and land in the correct place in every segment of a full-page
// scroll-stitch capture.

export interface AnnotationBox {
	number: number;
	x: number;
	y: number;
	width: number;
	height: number;
}

let annotationContainer: HTMLDivElement | null = null;

/**
 * Convert a getBoundingClientRect() (viewport-relative) into document-
 * absolute coordinates by adding the current scroll offset, and tag it
 * with a marker number.
 */
export function toAnnotationBox(
	rect: DOMRect,
	scrollX: number,
	scrollY: number,
	number: number,
): AnnotationBox {
	return {
		number,
		x: rect.left + scrollX,
		y: rect.top + scrollY,
		width: rect.width,
		height: rect.height,
	};
}

/** Draw numbered overlay boxes. Replaces any existing annotation overlay. */
export function showAnnotations(boxes: AnnotationBox[]): void {
	removeAnnotations();

	const container = document.createElement("div");
	container.id = "htrncontrol-annotations";
	container.setAttribute("data-htrncontrol-ignore", "true");
	Object.assign(container.style, {
		position: "absolute",
		top: "0",
		left: "0",
		width: "0",
		height: "0",
		pointerEvents: "none",
		zIndex: "2147483647",
	});

	for (const b of boxes) {
		const rect = document.createElement("div");
		Object.assign(rect.style, {
			position: "absolute",
			left: `${b.x}px`,
			top: `${b.y}px`,
			width: `${b.width}px`,
			height: `${b.height}px`,
			border: "2px solid #ef4444",
			boxSizing: "border-box",
			pointerEvents: "none",
		});

		const label = document.createElement("div");
		label.textContent = String(b.number);
		Object.assign(label.style, {
			position: "absolute",
			left: `${b.x}px`,
			top: `${b.y}px`,
			transform: "translateY(-100%)",
			background: "#ef4444",
			color: "#fff",
			font: "bold 12px sans-serif",
			padding: "1px 4px",
			borderRadius: "3px",
			pointerEvents: "none",
			whiteSpace: "nowrap",
		});

		container.appendChild(rect);
		container.appendChild(label);
	}

	document.body.appendChild(container);
	annotationContainer = container;
}

/** Remove the annotation overlay from the DOM. */
export function removeAnnotations(): void {
	if (annotationContainer?.parentNode) {
		annotationContainer.parentNode.removeChild(annotationContainer);
	}
	annotationContainer = null;
}
