/**
 * XPath Generator
 * Generates unique, short XPath expressions for DOM elements.
 */

/**
 * Generate the shortest unique XPath for an element.
 * Tries strategies in order of brevity:
 *   1. By ID (if unique)
 *   2. By tag + position
 *   3. Full path from root
 */
export function generateXPath(element: Element): string {
	// Try ID-based first
	const idXPath = generateIdXPath(element);
	if (idXPath) return idXPath;

	// Build path segments
	const segments: string[] = [];
	let current: Element | null = element;

	while (current && current !== document.documentElement) {
		const segment = generateSegment(current);
		segments.unshift(segment);
		current = current.parentElement;
	}

	return `/${segments.join("/")}`;
}

/**
 * Generate XPath using the element's ID (if unique on page).
 */
function generateIdXPath(element: Element): string | null {
	const id = element.id;
	if (!id) return null;

	// Verify uniqueness
	const matches = document.querySelectorAll(`#${CSS.escape(id)}`);
	if (matches.length !== 1) return null;

	// Use tag name if it matches, otherwise use //*[@id="..."]
	const tagMatches = document.querySelectorAll(
		`${element.tagName.toLowerCase()}#${CSS.escape(id)}`,
	);
	if (tagMatches.length === 1) {
		return `//${element.tagName.toLowerCase()}[@id="${id}"]`;
	}

	return `//*[@id="${id}"]`;
}

/**
 * Generate an XPath segment for a single element (without ancestors).
 * Uses tag name + position among siblings.
 */
function generateSegment(element: Element): string {
	const tag = element.tagName.toLowerCase();

	// Check if parent exists
	if (!element.parentElement) {
		return tag;
	}

	// Get siblings with same tag
	const siblings = Array.from(element.parentElement.children).filter(
		(s) => s.tagName === element.tagName,
	);

	if (siblings.length === 1) {
		// Unique tag under parent, no position needed
		return tag;
	}

	// Add position (1-based)
	const index = siblings.indexOf(element) + 1;
	return `${tag}[${index}]`;
}

/**
 * Generate a verbose but always-correct XPath (for debugging).
 */
export function generateVerboseXPath(element: Element): string {
	const segments: string[] = [];
	let current: Element | null = element;

	while (current) {
		if (current === document.documentElement) {
			segments.unshift(`/${current.tagName.toLowerCase()}`);
			break;
		}

		if (current === document.body) {
			segments.unshift(`/${current.tagName.toLowerCase()}`);
			current = current.parentElement;
			continue;
		}

		if (!current.parentElement) {
			segments.unshift(`/${current.tagName.toLowerCase()}`);
			break;
		}

		const parent = current.parentElement;
		const currentTag = current.tagName;
		const siblings = Array.from(parent.children).filter(
			(s) => s.tagName === currentTag,
		);

		const tag = current.tagName.toLowerCase();
		if (siblings.length > 1) {
			const index = siblings.indexOf(current) + 1;
			segments.unshift(`${tag}[${index}]`);
		} else {
			segments.unshift(tag);
		}

		current = current.parentElement;
	}

	return `/${segments.join("/")}`;
}
