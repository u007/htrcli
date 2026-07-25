import type { SnapshotNode, SnapshotNodeState } from "../types/commands";
import { assignRef } from "./refRegistry";

const PRESENTATION_ROLES = new Set(["none", "presentation"]);

const HEADING_TAGS = new Set(["h1", "h2", "h3", "h4", "h5", "h6"]);
const LANDMARK_TAGS = new Map<string, string>([
	["main", "main"],
	["nav", "navigation"],
	["header", "banner"],
	["footer", "contentinfo"],
	["aside", "complementary"],
	["form", "form"],
	["section", "region"],
	["article", "article"],
]);

/**
 * Build a best-effort accessibility-style snapshot tree from the current page.
 * The tree is intentionally shared across the extension and CDP transports so
 * snapshot works everywhere htrcli can execute DOM code.
 */
export function getSnapshot(): SnapshotNode {
	const rootChildren = snapshotChildren(
		document.body ?? document.documentElement,
	);
	return {
		role: "document",
		name: normalizeText(document.title),
		children: rootChildren.length > 0 ? rootChildren : undefined,
	};
}

function snapshotChildren(parent: ParentNode): SnapshotNode[] {
	const nodes: SnapshotNode[] = [];
	for (const child of Array.from(parent.children)) {
		nodes.push(...snapshotElement(child));
	}
	return nodes;
}

function snapshotElement(element: Element): SnapshotNode[] {
	if (isHidden(element)) {
		return [];
	}

	const children = snapshotChildren(element);
	const role = getRole(element);
	if (PRESENTATION_ROLES.has(role)) {
		return children;
	}

	const name = getAccessibleName(element, role);
	const value = getValue(element);
	const state = getState(element);
	const focusable = isFocusable(element);
	const interesting =
		role !== "generic" ||
		Boolean(name) ||
		Boolean(value) ||
		Boolean(state && Object.keys(state).length > 0) ||
		focusable;

	if (!interesting) {
		return children;
	}

	const node: SnapshotNode = { role };
	if (name) node.name = name;
	if (value) node.value = value;
	if (state && Object.keys(state).length > 0) node.state = state;
	if (shouldMintRef(element, role, focusable)) node.ref = assignRef(element);
	if (children.length > 0) node.children = children;
	return [node];
}

function shouldMintRef(
	element: Element,
	role: string,
	focusable: boolean,
): boolean {
	if (role === "text" || role === "document") return false;
	return focusable || role !== "generic" || isInteractiveElement(element);
}

function isInteractiveElement(element: Element): boolean {
	const tag = element.tagName.toLowerCase();
	if (tag === "button" || tag === "select" || tag === "textarea") return true;
	if (tag === "input") return true;
	if (tag === "summary") return true;
	if (tag === "a" && element.hasAttribute("href")) return true;
	if (tag === "option") return true;
	if (tag === "audio" || tag === "video")
		return element.hasAttribute("controls");
	return false;
}

function isFocusable(element: Element): boolean {
	if (!(element instanceof HTMLElement)) return false;
	if (element.hasAttribute("disabled")) return false;
	return element.tabIndex >= 0;
}

function isHidden(element: Element): boolean {
	if (!(element instanceof HTMLElement)) return false;
	if (element.hidden) return true;
	if (element.hasAttribute("inert")) return true;
	if (element.getAttribute("aria-hidden") === "true") return true;

	const style = window.getComputedStyle(element);
	return (
		style.display === "none" ||
		style.visibility === "hidden" ||
		style.visibility === "collapse"
	);
}

function getRole(element: Element): string {
	const explicit = normalizeRole(element.getAttribute("role"));
	if (explicit) return explicit;

	const tag = element.tagName.toLowerCase();
	if (HEADING_TAGS.has(tag)) return "heading";
	const landmark = LANDMARK_TAGS.get(tag);
	if (landmark) return landmark;

	switch (tag) {
		case "button":
			return "button";
		case "a":
			return element.hasAttribute("href") ? "link" : "generic";
		case "input":
			return getInputRole(element as HTMLInputElement);
		case "textarea":
			return "textbox";
		case "select":
			return (element as HTMLSelectElement).multiple ? "listbox" : "combobox";
		case "img":
			return "img";
		case "ul":
		case "ol":
			return "list";
		case "li":
			return "listitem";
		case "table":
			return "table";
		case "thead":
			return "rowgroup";
		case "tbody":
			return "rowgroup";
		case "tr":
			return "row";
		case "th":
			return "columnheader";
		case "td":
			return "cell";
		case "details":
			return "group";
		case "summary":
			return "button";
		case "dialog":
			return "dialog";
		default:
			return "generic";
	}
}

function getInputRole(input: HTMLInputElement): string {
	switch (input.type) {
		case "button":
		case "submit":
		case "reset":
		case "image":
			return "button";
		case "checkbox":
			return "checkbox";
		case "radio":
			return "radio";
		case "range":
			return "slider";
		case "search":
			return "searchbox";
		default:
			return "textbox";
	}
}

function normalizeRole(role: string | null): string {
	if (!role) return "";
	return role.trim().toLowerCase();
}

function getAccessibleName(element: Element, role: string): string {
	const ariaLabel = normalizeText(element.getAttribute("aria-label"));
	if (ariaLabel) return ariaLabel;

	const labelledBy = normalizeText(element.getAttribute("aria-labelledby"));
	if (labelledBy) {
		const ids = labelledBy.split(/\s+/).filter(Boolean);
		const parts: string[] = [];
		for (const id of ids) {
			const label = document.getElementById(id);
			const text = normalizeText(label?.textContent);
			if (text) parts.push(text);
		}
		if (parts.length > 0) return parts.join(" ");
	}

	const labelText = getAssociatedLabelText(element);
	if (labelText) return labelText;

	const alt = normalizeText(element.getAttribute("alt"));
	if (alt && (role === "img" || element.tagName.toLowerCase() === "img")) {
		return alt;
	}

	const title = normalizeText(element.getAttribute("title"));
	if (title) return title;

	const value = getValue(element);
	if (value) return value;

	const text =
		role === "generic"
			? directTextContent(element)
			: normalizeText(element.textContent);
	return text;
}

function getAssociatedLabelText(element: Element): string {
	if (!(element instanceof HTMLElement)) return "";

	if (
		element instanceof HTMLInputElement ||
		element instanceof HTMLTextAreaElement ||
		element instanceof HTMLSelectElement
	) {
		if (element.id) {
			for (const label of Array.from(document.querySelectorAll("label"))) {
				if (label instanceof HTMLLabelElement && label.htmlFor === element.id) {
					const labelText = normalizeText(label.textContent);
					if (labelText) return labelText;
				}
			}
		}

		const parentLabel = element.closest("label");
		const parentText = normalizeText(parentLabel?.textContent);
		if (parentText) return parentText;
	}

	return "";
}

function getValue(element: Element): string {
	if (
		element instanceof HTMLInputElement ||
		element instanceof HTMLTextAreaElement
	) {
		return normalizeText(element.value);
	}
	if (element instanceof HTMLSelectElement) {
		const option = element.options[element.selectedIndex];
		return normalizeText(option?.textContent);
	}
	if (element instanceof HTMLProgressElement) {
		return normalizeText(element.value.toString());
	}
	return "";
}

function getState(element: Element): SnapshotNodeState | undefined {
	const state: SnapshotNodeState = {};

	if (
		element instanceof HTMLInputElement ||
		element instanceof HTMLTextAreaElement ||
		element instanceof HTMLSelectElement ||
		element instanceof HTMLButtonElement
	) {
		if (element.disabled) state.disabled = true;
	}
	if (
		element instanceof HTMLInputElement ||
		element instanceof HTMLTextAreaElement
	) {
		if (element.readOnly) state.readonly = true;
	}

	const ariaDisabled = element.getAttribute("aria-disabled");
	if (ariaDisabled === "true") state.disabled = true;

	const ariaExpanded = element.getAttribute("aria-expanded");
	if (ariaExpanded === "true") state.expanded = true;

	const ariaSelected = element.getAttribute("aria-selected");
	if (ariaSelected === "true") state.selected = true;

	const ariaPressed = element.getAttribute("aria-pressed");
	if (ariaPressed === "true" || ariaPressed === "mixed") {
		state.pressed = ariaPressed === "mixed" ? "mixed" : true;
	}

	const ariaChecked = element.getAttribute("aria-checked");
	if (ariaChecked === "true" || ariaChecked === "mixed") {
		state.checked = ariaChecked === "mixed" ? "mixed" : true;
	}
	if (element instanceof HTMLInputElement) {
		if (element.type === "checkbox" || element.type === "radio") {
			if (element.checked) state.checked = true;
		}
	}
	if (element instanceof HTMLOptionElement && element.selected) {
		state.selected = true;
	}

	if (element === document.activeElement) state.focused = true;

	return Object.keys(state).length > 0 ? state : undefined;
}

function directTextContent(element: Element): string {
	const parts: string[] = [];
	for (const node of Array.from(element.childNodes)) {
		if (node.nodeType !== Node.TEXT_NODE) continue;
		const text = normalizeText(node.textContent);
		if (text) parts.push(text);
	}
	return parts.join(" ");
}

function normalizeText(text: string | null | undefined): string {
	return (text ?? "").replace(/\s+/g, " ").trim();
}
