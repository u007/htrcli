/**
 * CSS Selector Generator
 * Generates unique, robust CSS selectors for elements
 *
 * Priority order:
 * 1. #id - Use ID if present and unique
 * 2. [data-testid="value"] - Test IDs are stable
 * 3. [data-cy="value"] - Cypress test IDs
 * 4. [name="value"] - Form field names
 * 5. .class-name - Unique class names
 * 6. tag.class:nth-child(n) - Tag with class and position
 * 7. Full path from nearest ID ancestor
 */

// Attributes that are good for selectors (stable identifiers)
const STABLE_ATTRIBUTES = [
	"data-testid",
	"data-cy",
	"data-test",
	"data-qa",
	"name",
	"aria-label",
	"role",
];

// Classes that are likely auto-generated or unstable
const UNSTABLE_CLASS_PATTERNS = [
	/^css-/, // CSS-in-JS
	/^sc-/, // styled-components
	/^emotion-/, // Emotion
	/^_[a-z0-9]+$/i, // CSS modules
	/^[a-z]{1,2}[0-9]+$/i, // Minified classes
	/^jsx-/, // JSX styles
	/--/, // BEM modifiers often change
];

/**
 * Check if an element's ID is unique on the page
 */
function isUniqueId(_element: Element, id: string): boolean {
	return document.querySelectorAll(`#${CSS.escape(id)}`).length === 1;
}

/**
 * Check if a class name appears to be stable (not auto-generated)
 */
function isStableClass(className: string): boolean {
	return !UNSTABLE_CLASS_PATTERNS.some((pattern) => pattern.test(className));
}

/**
 * Get stable class names from an element
 */
function getStableClasses(element: Element): string[] {
	return Array.from(element.classList).filter(isStableClass);
}

/**
 * Check if a selector uniquely identifies an element
 */
function isUnique(selector: string): boolean {
	try {
		return document.querySelectorAll(selector).length === 1;
	} catch {
		return false;
	}
}

/**
 * Get the nth-child position of an element among its siblings
 */
function _getNthChildPosition(element: Element): number {
	if (!element.parentElement) return 1;
	const siblings = Array.from(element.parentElement.children);
	return siblings.indexOf(element) + 1;
}

/**
 * Get the nth-of-type position of an element among siblings with same tag
 */
function getNthOfTypePosition(element: Element): number {
	if (!element.parentElement) return 1;
	const tagName = element.tagName;
	const siblings = Array.from(element.parentElement.children).filter(
		(el) => el.tagName === tagName,
	);
	return siblings.indexOf(element) + 1;
}

/**
 * Try to generate a selector using ID
 */
function tryIdSelector(element: Element): string | null {
	const id = element.id;
	if (id && isUniqueId(element, id)) {
		return `#${CSS.escape(id)}`;
	}
	return null;
}

/**
 * Try to generate a selector using stable attributes
 */
function tryAttributeSelector(element: Element): string | null {
	for (const attr of STABLE_ATTRIBUTES) {
		const value = element.getAttribute(attr);
		if (value) {
			const selector = `[${attr}="${CSS.escape(value)}"]`;
			if (isUnique(selector)) {
				return selector;
			}
		}
	}
	return null;
}

/**
 * Try to generate a selector using tag and classes
 */
function tryClassSelector(element: Element): string | null {
	const tagName = element.tagName.toLowerCase();
	const stableClasses = getStableClasses(element);

	// Try tag + single class
	for (const className of stableClasses) {
		const selector = `${tagName}.${CSS.escape(className)}`;
		if (isUnique(selector)) {
			return selector;
		}
	}

	// Try tag + multiple classes
	if (stableClasses.length >= 2) {
		const classSelector = stableClasses
			.map((c) => `.${CSS.escape(c)}`)
			.join("");
		const selector = `${tagName}${classSelector}`;
		if (isUnique(selector)) {
			return selector;
		}
	}

	return null;
}

/**
 * Find the nearest ancestor with an ID
 */
function findNearestIdAncestor(
	element: Element,
): { ancestor: Element; path: Element[] } | null {
	const path: Element[] = [];
	let current = element.parentElement;

	while (
		current &&
		current !== document.body &&
		current !== document.documentElement
	) {
		if (current.id && isUniqueId(current, current.id)) {
			return { ancestor: current, path: path.reverse() };
		}
		path.push(current);
		current = current.parentElement;
	}

	return null;
}

/**
 * Build a path selector from an ancestor to the target
 */
function buildPathSelector(
	ancestor: Element,
	path: Element[],
	target: Element,
): string {
	const ancestorSelector = `#${CSS.escape(ancestor.id)}`;
	const parts: string[] = [ancestorSelector];

	for (const el of path) {
		parts.push(getSimpleSelector(el));
	}
	parts.push(getSimpleSelector(target));

	return parts.join(" > ");
}

/**
 * Get a simple selector for an element (tag, class, nth-child)
 */
function getSimpleSelector(element: Element): string {
	const tagName = element.tagName.toLowerCase();
	const stableClasses = getStableClasses(element);

	// Try tag + class first
	if (stableClasses.length > 0) {
		const selector = `${tagName}.${CSS.escape(stableClasses[0])}`;
		// Check if unique among siblings
		if (element.parentElement) {
			const matchingSiblings = element.parentElement.querySelectorAll(
				`:scope > ${selector}`,
			);
			if (matchingSiblings.length === 1) {
				return selector;
			}
		}
	}

	// Fall back to nth-of-type
	const position = getNthOfTypePosition(element);
	return `${tagName}:nth-of-type(${position})`;
}

/**
 * Build a full path selector from body/html to the element
 */
function buildFullPathSelector(element: Element): string {
	const path: string[] = [];
	let current: Element | null = element;

	while (
		current &&
		current !== document.body &&
		current !== document.documentElement
	) {
		path.unshift(getSimpleSelector(current));
		current = current.parentElement;
	}

	return path.join(" > ");
}

/**
 * Generate a unique CSS selector for an element
 */
export function generateSelector(element: Element): string {
	// 1. Try ID
	const idSelector = tryIdSelector(element);
	if (idSelector) return idSelector;

	// 2. Try stable attributes (data-testid, etc.)
	const attrSelector = tryAttributeSelector(element);
	if (attrSelector) return attrSelector;

	// 3. Try tag + classes
	const classSelector = tryClassSelector(element);
	if (classSelector) return classSelector;

	// 4. Try path from nearest ID ancestor
	const ancestorInfo = findNearestIdAncestor(element);
	if (ancestorInfo) {
		const pathSelector = buildPathSelector(
			ancestorInfo.ancestor,
			ancestorInfo.path,
			element,
		);
		if (isUnique(pathSelector)) {
			return pathSelector;
		}
	}

	// 5. Fall back to full path
	const fullPath = buildFullPathSelector(element);
	return fullPath;
}

/**
 * Verify that a selector correctly identifies the original element
 */
export function verifySelector(selector: string, element: Element): boolean {
	try {
		const found = document.querySelector(selector);
		return found === element;
	} catch {
		return false;
	}
}

/**
 * Generate a human-readable description of an element
 */
export function describeElement(element: Element): string {
	const tag = element.tagName.toLowerCase();

	// For buttons, links, and inputs, include text/label
	if (tag === "button" || tag === "a") {
		const text = element.textContent?.trim().slice(0, 50);
		if (text) return `"${text}" ${tag}`;
	}

	if (tag === "input") {
		const input = element as HTMLInputElement;
		const type = input.type || "text";
		const name = input.name || input.placeholder;
		if (name) return `${type} input "${name}"`;
		return `${type} input`;
	}

	if (tag === "select") {
		const select = element as HTMLSelectElement;
		const name = select.name;
		if (name) return `dropdown "${name}"`;
		return "dropdown";
	}

	if (tag === "textarea") {
		const textarea = element as HTMLTextAreaElement;
		const name = textarea.name || textarea.placeholder;
		if (name) return `textarea "${name}"`;
		return "textarea";
	}

	// For other elements, try aria-label or text
	const ariaLabel = element.getAttribute("aria-label");
	if (ariaLabel) return `${tag} "${ariaLabel}"`;

	const text = element.textContent?.trim().slice(0, 30);
	if (text) return `${tag} "${text}"`;

	return tag;
}
