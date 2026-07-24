/**
 * In-page element ref registry.
 *
 * Maintains a Map<string, Element> in the content-script world so that refs
 * like "@e1" survive across CLI calls as long as the page does. On full-page
 * navigation the entire registry is lost (cleared in document_start).
 *
 * Resolution checks isConnected so a detached (SPA-navigated) element produces
 * an explicit stale-ref error instead of silently matching.
 */

// Map from ref id to element. Shared across all commands within a page lifetime.
const refToEl = new Map<string, Element>();

// Monotonically increasing counter for the next ref id.
let nextRef = 0;

/**
 * Assign a new @eN ref to an element. If the element already has a ref (same
 * object identity), returns the existing ref id — no duplicate handles.
 */
export function assignRef(el: Element): string {
	// Check if this element already has a ref by scanning values.
	for (const [refId, existing] of refToEl) {
		if (existing === el) {
			return refId;
		}
	}
	nextRef++;
	const refId = `@e${nextRef}` as const;
	refToEl.set(refId, el);
	return refId;
}

/**
 * Resolve a ref id back to its element. Throws an explicit stale-ref error if
 * the id was never minted (unknown / wrong page) or the element has since
 * been detached from the document.
 */
export function resolveRef(refId: string): Element {
	const el = refToEl.get(refId);
	if (!el) {
		throw new Error(
			`stale ref: ${refId} is not known on this page (it may have navigated or the ref was minted elsewhere)`,
		);
	}
	if (!el.isConnected) {
		refToEl.delete(refId);
		throw new Error(
			`stale ref: ${refId} points to an element that is no longer in the document (page re-rendered or navigated)`,
		);
	}
	return el;
}

/** Drop all refs. Called on full page navigation reset. */
export function clearRefs(): void {
	refToEl.clear();
	nextRef = 0;
}

/** Number of currently-held refs (diagnostics / tests). */
export function refCount(): number {
	return refToEl.size;
}
