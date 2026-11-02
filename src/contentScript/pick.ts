/**
 * Option picker for the `pick` action: chooses the option whose text matches a
 * wanted string, in a native <select> or a custom combobox (react-select style,
 * async autocomplete). Port of htrcli/internal/cdp/pick.go and formkit.js. The
 * extension transport has no trusted input, so every step is a synthetic event.
 */
import { resolveKey } from "../utils/keyMap";
import { dispatchClickSequence } from "./clickSequence";
import { isInputElement, isSelectElement } from "./elementKind";

/** How long to poll for a matching option after typing (async lists answer late). */
const OPTIONS_WAIT_MS = 3000;
/** How long to wait for the widget to display the chosen option. */
const VERIFY_WAIT_MS = 2000;
const POLL_MS = 100;
/** Cap on options listed in an error message. */
const LIST_MAX = 15;

export interface PickResult {
	/** The control's displayed or selected value after the pick. */
	value: string;
	/** The option text that was chosen. */
	matched: string;
}

export async function pickOption(
	element: Element,
	want: string,
): Promise<PickResult> {
	if (want.trim() === "") {
		throw new Error("pick: option text cannot be empty");
	}
	if (isSelectElement(element)) {
		return pickSelect(element, want);
	}
	return pickCombobox(element, want);
}

// ─── Ranking (pure) ───────────────────────────────────────────────

function normalize(text: string): string {
	return text.replace(/\s+/g, " ").trim().toLowerCase();
}

/**
 * 3 for an exact match, 2 for a prefix, 1 for a substring, 0 for no match.
 * Case-insensitive and whitespace-normalised.
 */
export function optionRank(option: string, want: string): number {
	const o = normalize(option);
	const w = normalize(want);
	if (w === "") return 0;
	if (o === w) return 3;
	if (o.startsWith(w)) return 2;
	if (o.includes(w)) return 1;
	return 0;
}

/**
 * Index of the unique best-ranked option. `index` is -1 when nothing matches.
 * When several options share the best rank, `tied` lists them: the caller must
 * report that, not guess.
 */
export function bestOption(
	options: string[],
	want: string,
): { index: number; tied: string[] | null } {
	let bestRank = 0;
	let best: number[] = [];
	options.forEach((option, i) => {
		const rank = optionRank(option, want);
		if (rank === 0) return;
		if (rank > bestRank) {
			bestRank = rank;
			best = [i];
		} else if (rank === bestRank) {
			best.push(i);
		}
	});
	if (best.length === 0) return { index: -1, tied: null };
	if (best.length > 1) return { index: -1, tied: best.map((i) => options[i]) };
	return { index: best[0], tied: null };
}

function sameOptions(a: string[] | null, b: string[]): boolean {
	return a !== null && a.length === b.length && a.every((v, i) => v === b[i]);
}

function listOptions(options: string[]): string {
	if (options.length === 0) return "(no options visible)";
	const shown = options.slice(0, LIST_MAX).map((o) => JSON.stringify(o));
	const more =
		options.length > LIST_MAX ? ` … and ${options.length - LIST_MAX} more` : "";
	return shown.join(", ") + more;
}

// ─── Native <select> ──────────────────────────────────────────────

interface SelectOption {
	value: string;
	text: string;
}

/**
 * Resolve want to one option: an exact value match first, else the best text
 * match. Throws on no match or a tie, listing what was seen.
 */
export function resolveSelectOption(
	options: SelectOption[],
	want: string,
): SelectOption {
	const byValue = options.filter((o) => o.value === want);
	if (byValue.length === 1) return byValue[0];
	const best = bestOption(
		options.map((o) => o.text),
		want,
	);
	if (best.tied) {
		throw new Error(
			`pick: ${JSON.stringify(want)} is ambiguous: it matches ${listOptions(best.tied)} equally well`,
		);
	}
	if (best.index < 0) {
		throw new Error(
			`pick: no option matches ${JSON.stringify(want)}; available: ${listOptions(options.map((o) => o.text))}`,
		);
	}
	return options[best.index];
}

function pickSelect(select: HTMLSelectElement, want: string): PickResult {
	const options = Array.from(select.options).map((o) => ({
		value: o.value,
		text: textOf(o),
	}));
	const chosen = resolveSelectOption(options, want);
	select.focus();
	select.value = chosen.value;
	select.dispatchEvent(new Event("input", { bubbles: true }));
	select.dispatchEvent(new Event("change", { bubbles: true }));
	if (select.value !== chosen.value) {
		throw new Error(
			`pick: selected ${JSON.stringify(chosen.text)} but the <select> holds value ${JSON.stringify(select.value)}`,
		);
	}
	return { value: select.value, matched: chosen.text };
}

// ─── Custom combobox ──────────────────────────────────────────────

function textOf(element: Element): string {
	return (element.textContent || "").replace(/\s+/g, " ").trim();
}

function describe(element: Element): string {
	if (element.id) return `${element.tagName.toLowerCase()}#${element.id}`;
	const firstClass =
		typeof element.className === "string"
			? element.className.trim().split(/\s+/)[0]
			: "";
	return element.tagName.toLowerCase() + (firstClass ? `.${firstClass}` : "");
}

function isShown(element: Element): boolean {
	const view = element.ownerDocument.defaultView;
	if (!view) return false;
	const rect = element.getBoundingClientRect();
	if (rect.width === 0 || rect.height === 0) return false;
	const style = view.getComputedStyle(element);
	return style.display !== "none" && style.visibility !== "hidden";
}

/** The <input> that takes the typed query: the element itself, or one inside it. */
function comboInput(element: Element): HTMLInputElement {
	if (isInputElement(element)) return element;
	const inner =
		element.querySelector<HTMLInputElement>('input[role="combobox"]') ??
		element.querySelector<HTMLInputElement>("input");
	if (inner) return inner;
	throw new Error(
		`pick: ${describe(element)} is not a combobox: it is not an <input> and has no <input> inside`,
	);
}

/** Listbox named by aria-controls / aria-owns on the input or its combobox ancestor. */
function comboListbox(input: HTMLInputElement): Element | null {
	const hosts = [input, input.closest('[role="combobox"]')];
	for (const host of hosts) {
		if (!host) continue;
		for (const attr of ["aria-controls", "aria-owns"]) {
			for (const id of (host.getAttribute(attr) ?? "").split(/\s+/)) {
				if (!id) continue;
				const listbox = input.ownerDocument.getElementById(id);
				if (listbox) return listbox;
			}
		}
	}
	return null;
}

/**
 * The nearest ancestor holding an open listbox or menu. Closed menus that
 * widgets keep in the DOM are ignored; two open ones are ambiguous.
 */
function comboContainerListbox(input: HTMLInputElement): Element | null {
	for (
		let container: HTMLElement | null = input.parentElement;
		container;
		container = container.parentElement
	) {
		const open = Array.from(
			container.querySelectorAll('[role="listbox"], [role="menu"]'),
		).filter(isShown);
		if (open.length === 1) return open[0];
		if (open.length > 1) {
			throw new Error(
				`${describe(input)}: ${open.length} open listboxes or menus share its container; cannot tell which one belongs to this combobox`,
			);
		}
	}
	return null;
}

function comboOptionElements(input: HTMLInputElement): HTMLElement[] {
	const listbox = comboListbox(input) ?? comboContainerListbox(input);
	if (!listbox) {
		throw new Error(
			`${describe(input)}: no listbox is associated with this combobox (no aria-controls/aria-owns, and no listbox or menu in its container); not guessing its options`,
		);
	}
	return Array.from(
		listbox.querySelectorAll<HTMLElement>('[role="option"]'),
	).filter(isShown);
}

/** What the widget displays as selected, and where that was read from. */
function displayedValue(input: HTMLInputElement): {
	text: string;
	from: string;
} {
	// react-select keeps the typed query in the input, so the selected value is
	// read from the control's own value container, never from the input.
	const scope =
		input.closest('.select__value-container, [class*="__value-container"]') ??
		input.closest('.select__control, [class*="__control"]');
	if (scope) {
		const single = scope.querySelector(
			'.select__single-value, [class*="singleValue"], [class*="single-value"]',
		);
		if (single) {
			return {
				text: textOf(single),
				from: `${describe(single)} in ${describe(scope)}`,
			};
		}
		const multi = Array.from(
			scope.querySelectorAll(
				'.select__multi-value__label, [class*="multi-value__label"]',
			),
		);
		if (multi.length > 0) {
			return {
				text: multi.map(textOf).join(", "),
				from: `multi-value labels in ${describe(scope)}`,
			};
		}
		return {
			text: "",
			from: `no selected-value element in ${describe(scope)}`,
		};
	}
	if (input.value) {
		return { text: input.value, from: `value of ${describe(input)}` };
	}
	return {
		text: input.getAttribute("aria-valuetext") ?? "",
		from: `aria-valuetext of ${describe(input)}`,
	};
}

/** Set the input's value through the native setter, then tell the framework. */
function setQuery(input: HTMLInputElement, value: string): void {
	const setter = Object.getOwnPropertyDescriptor(
		HTMLInputElement.prototype,
		"value",
	)?.set;
	if (!setter) {
		throw new Error("pick: HTMLInputElement value setter is unavailable");
	}
	setter.call(input, value);
	input.dispatchEvent(new Event("input", { bubbles: true }));
}

function clearQuery(input: HTMLInputElement): void {
	if (input.value !== "") setQuery(input, "");
}

function pressKey(target: HTMLElement, key: string): void {
	const descriptor = resolveKey(key);
	const init: KeyboardEventInit = {
		bubbles: true,
		cancelable: true,
		key: descriptor.key,
		code: descriptor.code,
		keyCode: descriptor.windowsVirtualKeyCode,
		which: descriptor.windowsVirtualKeyCode,
	};
	target.dispatchEvent(new KeyboardEvent("keydown", init));
	target.dispatchEvent(new KeyboardEvent("keyup", init));
}

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

interface Settle {
	/** Index of the chosen option, or -1. */
	choice: number;
	tied: string[] | null;
	/** True once the option list is conclusive. */
	done: boolean;
}

/**
 * Whether one poll of the option list is conclusive. An exact match is taken
 * at once. A weaker match or a tie is only final once the list is unchanged
 * across two polls, so a half-loaded async list cannot win over the exact
 * option still loading.
 */
function settle(
	previous: string[] | null,
	options: string[],
	want: string,
): Settle {
	const best = bestOption(options, want);
	if (best.index >= 0) {
		return {
			choice: best.index,
			tied: null,
			done:
				optionRank(options[best.index], want) === 3 ||
				sameOptions(previous, options),
		};
	}
	if (best.tied) {
		return {
			choice: -1,
			tied: best.tied,
			done: sameOptions(previous, options),
		};
	}
	return { choice: -1, tied: null, done: false };
}

async function pollOptions(
	input: HTMLInputElement,
	want: string,
): Promise<{ options: string[]; verdict: Settle }> {
	const deadline = Date.now() + OPTIONS_WAIT_MS;
	let previous: string[] | null = null;
	for (;;) {
		const options = comboOptionElements(input).map(textOf);
		const verdict = settle(previous, options, want);
		if (verdict.done || Date.now() >= deadline) return { options, verdict };
		previous = options;
		await sleep(POLL_MS);
	}
}

/**
 * Opens the menu with no query and reads what it offers, for the no-match
 * error. Closes the menu again before returning.
 */
async function unfilteredOptions(input: HTMLInputElement): Promise<string[]> {
	clearQuery(input);
	pressKey(input, "ArrowDown");
	const deadline = Date.now() + VERIFY_WAIT_MS;
	let options: string[] = [];
	for (;;) {
		options = comboOptionElements(input).map(textOf);
		if (options.length > 0 || Date.now() >= deadline) break;
		await sleep(POLL_MS);
	}
	pressKey(input, "Escape");
	return options;
}

async function verifyShown(
	input: HTMLInputElement,
	chosen: string,
	options: string[],
): Promise<PickResult> {
	const want = normalize(chosen);
	const deadline = Date.now() + VERIFY_WAIT_MS;
	for (;;) {
		const shown = displayedValue(input);
		// Verify against the option that was clicked, not the requested text: an
		// empty or partial request would match any stale label.
		if (want !== "" && normalize(shown.text).includes(want)) {
			return { value: shown.text, matched: chosen };
		}
		if (Date.now() >= deadline) {
			throw new Error(
				`pick: clicked option ${JSON.stringify(chosen)} but after ${VERIFY_WAIT_MS}ms the combobox shows ${JSON.stringify(shown.text)} (read from ${shown.from}), which does not contain ${JSON.stringify(chosen)}; options were: ${listOptions(options)}`,
			);
		}
		await sleep(POLL_MS);
	}
}

async function chooseOption(
	input: HTMLInputElement,
	want: string,
): Promise<PickResult> {
	const { options, verdict } = await pollOptions(input, want);
	if (verdict.tied) {
		throw new Error(
			`pick: ${JSON.stringify(want)} is ambiguous: it matches ${listOptions(verdict.tied)} equally well; pass the full option text`,
		);
	}
	if (verdict.choice < 0) {
		if (options.length === 0) {
			// The typed text filtered everything out: show the unfiltered menu so
			// the caller can see what the widget actually offers.
			const available = await unfilteredOptions(input);
			throw new Error(
				`pick: no option matching ${JSON.stringify(want)} appeared within ${OPTIONS_WAIT_MS}ms; the menu without a query offers: ${listOptions(available)}`,
			);
		}
		throw new Error(
			`pick: no option matching ${JSON.stringify(want)} appeared within ${OPTIONS_WAIT_MS}ms; available: ${listOptions(options)}`,
		);
	}
	const chosen = options[verdict.choice];
	// Re-find the option by text: the list may re-render between the poll that
	// chose it and this click.
	const matches = comboOptionElements(input).filter(
		(o) => normalize(textOf(o)) === normalize(chosen),
	);
	if (matches.length !== 1) {
		throw new Error(
			`pick: option ${JSON.stringify(chosen)} is no longer uniquely present (${matches.length} matches)`,
		);
	}
	const option = matches[0];
	option.scrollIntoView({
		block: "nearest",
		inline: "nearest",
		behavior: "instant",
	});
	const rect = option.getBoundingClientRect();
	dispatchClickSequence(
		option,
		rect.left + rect.width / 2,
		rect.top + rect.height / 2,
		"left",
		1,
	);
	return verifyShown(input, chosen, options);
}

async function pickCombobox(
	element: Element,
	want: string,
): Promise<PickResult> {
	const input = comboInput(element);
	input.scrollIntoView({
		block: "center",
		inline: "center",
		behavior: "instant",
	});
	clearQuery(input);
	input.focus();
	if (input.ownerDocument.activeElement !== input) {
		throw new Error(
			`pick: could not focus the combobox input at ${describe(input)}`,
		);
	}
	pressKey(input, "ArrowDown");
	setQuery(input, want);
	try {
		return await chooseOption(input, want);
	} catch (error) {
		// Close the menu on every failure so it cannot overlay the next widget.
		pressKey(input, "Escape");
		throw error;
	}
}
