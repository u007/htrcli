/**
 * Structured data extraction from the page.
 *
 * Two strategies:
 *   - `extractTable`: reads an HTML `<table>` (or the first table inside the
 *     matched container) and returns `{ columns, rows }` where each row is an
 *     object keyed by the header text.
 *   - `extractList`: reads a list of similar elements and returns
 *     `{ items }`. With `fields` (a record of name -> relative CSS selector)
 *     each item becomes an object with one value per field; without fields
 *     each item is `{ text }`.
 *
 * Both are read-only and never mutate the page.
 */

import { isRendered } from "./elementFinder";

export interface ExtractedTable {
	columns: string[];
	rows: Record<string, string>[];
	rowCount: number;
}

export interface ExtractedList {
	items: Record<string, string>[];
	count: number;
}

function textOf(el: Element | null): string {
	if (!el) return "";
	return (el.textContent ?? "").trim();
}

function firstTable(root: Element): HTMLTableElement | null {
	if (root instanceof HTMLTableElement) return root;
	const found = root.querySelector("table");
	return found instanceof HTMLTableElement ? found : null;
}

export function extractTable(root: Element): ExtractedTable {
	const table = firstTable(root);
	if (!table) {
		return { columns: [], rows: [], rowCount: 0 };
	}
	// Header: a <thead>'s first row, or — only when the table has no usable
	// <thead> — a first row whose cells are ALL <th>. A row of <td>s is DATA:
	// treating it as a header silently drops that row's values and turns its
	// first cell into a column name. Remember WHICH row supplied the header
	// so it can be excluded from the data rows.
	let headerCells: Element[] = [];
	let headerRow: Element | null = null;
	const thead = table.querySelector("thead");
	if (thead) {
		const tr = thead.querySelector("tr");
		if (tr) {
			headerCells = Array.from(tr.querySelectorAll("th, td"));
			headerRow = tr;
		}
	}
	if (headerCells.length === 0) {
		// Look at the first row OUTSIDE the thead: a <thead> that is present
		// but empty must not shadow a header row in the body.
		const firstTr = Array.from(table.querySelectorAll("tr")).find(
			(tr) => !thead?.contains(tr),
		);
		const cells = firstTr ? Array.from(firstTr.querySelectorAll("td, th")) : [];
		// <th> is the explicit HTML signal for a header cell. Only a row made
		// entirely of <th> is a header row; a <td> row is data.
		if (
			firstTr &&
			cells.length > 0 &&
			cells.every((c) => c.tagName?.toUpperCase() === "TH")
		) {
			headerCells = cells;
			headerRow = firstTr;
		}
	}

	const bodyRows: Element[] = [];
	const tbody = table.querySelector("tbody");
	if (tbody) {
		// A bare <tr> table is wrapped in a tbody by the HTML parser, so the
		// header row can sit inside it. Exclude it here too.
		for (const tr of Array.from(tbody.querySelectorAll("tr"))) {
			if (tr === headerRow) continue;
			bodyRows.push(tr);
		}
	} else {
		// Exclude the header row wherever it actually lives. A table with a
		// <thead> but no <tbody> keeps its header in a different subtree, so
		// skipping a fixed index would emit it as data.
		for (const tr of Array.from(table.querySelectorAll("tr"))) {
			if (tr === headerRow) continue;
			if (thead?.contains(tr)) continue;
			bodyRows.push(tr);
		}
	}

	// Duplicate header names would overwrite each other in the row object and
	// silently drop a column's value, so make them unique.
	const seenNames = new Map<string, number>();
	let columns: string[];
	if (headerCells.length > 0) {
		columns = headerCells.map((c, i) => {
			const base = textOf(c) || `col${i + 1}`;
			const seen = seenNames.get(base) ?? 0;
			seenNames.set(base, seen + 1);
			return seen === 0 ? base : `${base}_${seen + 1}`;
		});
	} else {
		// No header row: the column COUNT comes from the widest data row.
		const width = bodyRows.reduce(
			(max, tr) => Math.max(max, tr.querySelectorAll("td, th").length),
			0,
		);
		columns = Array.from({ length: width }, (_, i) => `col${i + 1}`);
	}

	const rows: Record<string, string>[] = [];
	for (const tr of bodyRows) {
		const cells = Array.from(tr.querySelectorAll("td, th"));
		if (cells.length === 0) continue;
		const row: Record<string, string> = {};
		cells.forEach((cell, i) => {
			const col = columns[i] ?? `col${i + 1}`;
			row[col] = textOf(cell);
		});
		rows.push(row);
	}
	return { columns, rows, rowCount: rows.length };
}

export function extractList(
	root: Element,
	fields?: Record<string, string>,
): ExtractedList {
	// This helper's contract is "root is a container": it has always taken the
	// root's children as the items, so keep that behaviour explicit.
	return extractListFrom([root], fields, { container: true });
}

/**
 * Extract a list from a set of matched roots.
 *
 * A selector can either name the CONTAINER (`ul#items`) or the ITEMS
 * themselves (`#items li`, `tr.row`). When several elements match, they are
 * the items: taking only the first match and looking for children inside it
 * silently returns zero items for a selector that already named them.
 *
 * With a single root the container behaviour applies: direct <li> children
 * for a <ul>/<ol>, otherwise the direct element children.
 */
/** Semantic list/table containers that a single match may legitimately name. */
const CONTAINER_TAGS = new Set([
	"ul",
	"ol",
	"table",
	"tbody",
	"thead",
	"dl",
	"select",
]);

/** True when the element is a semantic list/table container, not an item. */
function isContainerElement(el: Element): boolean {
	return CONTAINER_TAGS.has(el.tagName.toLowerCase());
}

export interface ExtractListOptions {
	/** Force container semantics: the match's children are the items. */
	container?: boolean;
	/**
	 * Drop items that `checkVisibility` reports as hidden, e.g. ones inside a
	 * `display:none` subtree (an SPA that keeps its previous view in the DOM).
	 *
	 * Opt-in on purpose: it needs `Element.checkVisibility`, which older hosts
	 * lack, so defaulting this on would fail every extraction there. Without
	 * the API this throws instead of keeping hidden items.
	 */
	visibleOnly?: boolean;
}

/**
 * Extract a list from a set of matched roots.
 *
 * A selector can either name the CONTAINER (`ul#items`) or the ITEMS
 * themselves (`#items li`, `tr.row`). Inferring that from the match COUNT is
 * unreliable: one matching `.row` would be treated as a container and expand
 * into its own child spans, so a single-row result came back as several
 * broken items. The intent is therefore explicit:
 *   - several matches  -> every match is an item;
 *   - one match        -> an item, unless it is a semantic list/table
 *                         container (ul/ol/table/…), which expands;
 *   - `container: true`-> always expand the match(es) into their children.
 */
export function extractListFrom(
	roots: Element[],
	fields?: Record<string, string>,
	options?: ExtractListOptions,
): ExtractedList {
	let items: Element[] = [];
	if (options?.container) {
		items = roots.flatMap((root) => listItemsOf(root));
	} else if (roots.length > 1) {
		// Several matches: each match is one item.
		items = roots;
	} else if (roots.length === 1) {
		items = isContainerElement(roots[0]) ? listItemsOf(roots[0]) : [roots[0]];
	}

	if (options?.visibleOnly) {
		items = items.filter(isRendered);
	}

	// Split and validate every field selector once, before the item loop, so a
	// zero-item result still rejects a malformed selector.
	const fieldPlans =
		fields && Object.keys(fields).length > 0
			? Object.entries(fields).map(([name, sel]) => ({
					name,
					sel,
					alternatives: alternativesOf(name, sel),
				}))
			: [];

	const out: Record<string, string>[] = [];
	for (const item of items) {
		if (fieldPlans.length > 0) {
			const row: Record<string, string> = {};
			for (const plan of fieldPlans) {
				row[plan.name] = readFieldValue(
					item,
					plan.alternatives,
					plan.sel,
					plan.name,
				);
			}
			out.push(row);
		} else {
			out.push({ text: textOf(item) });
		}
	}
	return { items: out, count: out.length };
}

/**
 * Read one field of an item.
 *
 * The field selector has three forms:
 *   ".name"          a descendant's TEXT
 *   "@data-due"      the ITEM's own attribute
 *   "@.link@href"    a DESCENDANT's attribute (selector@attribute)
 *   "[data-due]"     an attribute value (the first match: a descendant, else
 *                    the item itself)
 *
 * Several forms can be offered as alternatives with `||`, e.g.
 * "span.yP||span.zF": the first alternative that yields a NON-EMPTY value
 * wins. A `||` inside a quoted attribute value is not a separator.
 *
 * A plain selector matches a DESCENDANT only. When the selector also matches
 * the item itself (e.g. "[data-due]" on the <li> that carries it), the
 * descendant lookup finds nothing and the field comes back empty — the
 * item's own value is the one the caller meant. So the item itself is
 * consulted when no descendant matched.
 */
/** Split a field selector into its alternatives, rejecting an empty one. */
function alternativesOf(name: string, sel: string): string[] {
	const alternatives = splitFieldAlternatives(sel);
	if (alternatives.length === 0) {
		throw new Error(`invalid selector for field "${name}": ${sel} (empty)`);
	}
	return alternatives;
}

function readFieldValue(
	item: Element,
	alternatives: string[],
	sel: string,
	name: string,
): string {
	// A malformed selector throws a bare DOMException that names neither the
	// field nor the caller's intent, and aborts every other field with it.
	// Re-throw with the field name so the caller can fix the typo.
	try {
		// The first alternative that yields a NON-EMPTY value wins. That is the
		// point of the fallback: the earlier form does not exist on this row
		// (Gmail renders the sender in span.yP for some rows and span.zF for
		// others), and an empty string is not a value worth reporting.
		for (const alternative of alternatives) {
			const value = readFieldValueUnsafe(item, alternative);
			if (value !== "") return value;
		}
		return "";
	} catch (err) {
		const detail = err instanceof Error ? err.message : String(err);
		// Preserve the cause chain and the original stack for the caller.
		throw new Error(
			`invalid selector for field "${name}": ${sel} (${detail})`,
			{
				cause: err,
			},
		);
	}
}

/**
 * Split a field selector into alternatives on `||`, ignoring separators that
 * sit inside a quoted attribute value.
 *
 * `||` rather than `|` because a single pipe is live CSS: `[lang|=en]` would be
 * torn in half. Even `||` can appear inside a quoted value (`[title="a||b"]`),
 * so quotes are tracked and their contents are never treated as a separator.
 */
function splitFieldAlternatives(sel: string): string[] {
	const parts: string[] = [];
	let current = "";
	let quote = "";
	for (let i = 0; i < sel.length; i++) {
		const ch = sel[i];
		if (quote !== "") {
			current += ch;
			if (ch === "\\" && i + 1 < sel.length) {
				// An escaped character (`\"`) does not close the quote.
				i++;
				current += sel[i];
			} else if (ch === quote) {
				quote = "";
			}
			continue;
		}
		if (ch === '"' || ch === "'") {
			quote = ch;
			current += ch;
			continue;
		}
		if (ch === "|" && sel[i + 1] === "|") {
			parts.push(current);
			current = "";
			i++;
			continue;
		}
		current += ch;
	}
	parts.push(current);
	return parts.map((part) => part.trim()).filter((part) => part !== "");
}

function readFieldValueUnsafe(item: Element, sel: string): string {
	const attr = parseAttributeField(sel);
	if (attr) {
		const el = findFieldElement(item, attr.selector);
		if (!el) return "";
		return el.getAttribute(attr.attribute) ?? "";
	}
	// A pure attribute selector asks for that attribute's VALUE, not the
	// matched element's text: "[data-due]" on a task row means the due date,
	// and returning the whole row's text would be useless.
	const pureAttr = parsePureAttributeSelector(sel);
	if (pureAttr) {
		const el = findFieldElement(item, "");
		const found = el ? el.querySelector(sel) : null;
		const target = found ?? (el?.matches(sel) ? el : null);
		return target ? (target.getAttribute(pureAttr) ?? "") : "";
	}
	// Plain selector: a descendant wins, because a field named ".title" means
	// "the title inside this row", not "the row that has class title".
	const found = item.querySelector(sel);
	if (found) return textOf(found);
	if (item.matches(sel)) return textOf(item);
	return "";
}

/**
 * The attribute a field selector asks for, or null when the selector is a
 * plain text selector.
 *
 * "@data-due"          -> { selector: "", attribute: "data-due" }
 * "@.link@href"        -> { selector: ".link", attribute: "href" }
 */
function parseAttributeField(
	sel: string,
): { selector: string; attribute: string } | null {
	if (!sel.startsWith("@")) return null;
	const rest = sel.slice(1);
	const at = rest.lastIndexOf("@");
	if (at === -1) {
		return { selector: "", attribute: rest };
	}
	return { selector: rest.slice(0, at), attribute: rest.slice(at + 1) };
}

/**
 * The attribute name of a PURE attribute selector ("[data-due]"), or null
 * for any other selector. A compound selector ("a[href]") is not pure: it
 * can match a link whose text is what the caller wants.
 */
function parsePureAttributeSelector(sel: string): string | null {
	const m = /^\[([A-Za-z0-9_:.-]+)\]$/.exec(sel);
	return m ? m[1] : null;
}

/**
 * The element an attribute field refers to: the item itself when no
 * descendant selector is given, else the first descendant matching it.
 */
function findFieldElement(item: Element, selector: string): Element | null {
	if (selector === "") return item;
	return item.querySelector(selector);
}

/**
 * The items contained in a single container element.
 *
 * Direct <li> children for a <ul>/<ol>, otherwise the direct element
 * children. A container with no element children yields no items.
 */
function listItemsOf(root: Element): Element[] {
	if (root instanceof HTMLUListElement || root instanceof HTMLOListElement) {
		const lis = Array.from(root.children).filter(
			(c) => c instanceof HTMLLIElement,
		);
		if (lis.length > 0) return lis;
		return Array.from(root.children);
	}
	const lis = root.querySelectorAll(":scope > li");
	if (lis.length > 0) {
		return Array.from(lis);
	}
	return Array.from(root.children);
}
