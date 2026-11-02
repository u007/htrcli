/**
 * Realm-safe element type guards.
 *
 * An element in an `<iframe>`'s document belongs to that frame's JavaScript
 * realm, so `element instanceof HTMLInputElement` (the top window's constructor)
 * is false for it. These guards check the XHTML namespace and local name, which
 * hold in every realm.
 */

const XHTML_NAMESPACE = "http://www.w3.org/1999/xhtml";

function isXhtmlNamed(element: Element, localName: string): boolean {
	return (
		element.namespaceURI === XHTML_NAMESPACE && element.localName === localName
	);
}

export function isHTMLElement(element: Element): element is HTMLElement {
	return element.namespaceURI === XHTML_NAMESPACE;
}

export function isIFrameElement(
	element: Element,
): element is HTMLIFrameElement {
	return isXhtmlNamed(element, "iframe");
}

export function isInputElement(element: Element): element is HTMLInputElement {
	return isXhtmlNamed(element, "input");
}

export function isTextAreaElement(
	element: Element,
): element is HTMLTextAreaElement {
	return isXhtmlNamed(element, "textarea");
}

export function isSelectElement(
	element: Element,
): element is HTMLSelectElement {
	return isXhtmlNamed(element, "select");
}

export function isButtonElement(
	element: Element,
): element is HTMLButtonElement {
	return isXhtmlNamed(element, "button");
}

export function isFieldSetElement(
	element: Element,
): element is HTMLFieldSetElement {
	return isXhtmlNamed(element, "fieldset");
}

export function isLabelElement(element: Element): element is HTMLLabelElement {
	return isXhtmlNamed(element, "label");
}

export function isOptionElement(
	element: Element,
): element is HTMLOptionElement {
	return isXhtmlNamed(element, "option");
}

export function isProgressElement(
	element: Element,
): element is HTMLProgressElement {
	return isXhtmlNamed(element, "progress");
}
