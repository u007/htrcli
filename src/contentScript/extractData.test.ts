import { beforeEach, describe, expect, it } from "bun:test";
import { extractList, extractListFrom, extractTable } from "./extractData";

describe("extractTable", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("reads a table with thead and tbody", () => {
		document.body.innerHTML = `
			<table id="t">
				<thead><tr><th>Name</th><th>Age</th></tr></thead>
				<tbody>
					<tr><td>Alice</td><td>30</td></tr>
					<tr><td>Bob</td><td>25</td></tr>
				</tbody>
			</table>`;
		const _el = document.getElementById("t");
		if (!_el) throw new Error("missing element");
		const root = _el;
		const r = extractTable(root);
		expect(r.columns).toEqual(["Name", "Age"]);
		expect(r.rowCount).toBe(2);
		expect(r.rows).toEqual([
			{ Name: "Alice", Age: "30" },
			{ Name: "Bob", Age: "25" },
		]);
	});

	it("does not infer a header from a td-only first row when there is no thead", () => {
		// A row of <td>s is DATA. Treating it as a header silently drops that
		// row's values and turns its first cell into a column name — the exact
		// data loss the Sheets grid fixture hit (benchmark scenario S06).
		// Explicit <tbody>: browsers wrap bare <tr> rows in a tbody, so the
		// row lands inside the tbody branch. happy-dom does not insert the
		// tbody itself, so the fixture must include it.
		document.body.innerHTML = `
			<table id="t">
				<tbody>
					<tr><td>Name</td><td>Age</td></tr>
					<tr><td>Carol</td><td>40</td></tr>
				</tbody>
			</table>`;
		const _el = document.getElementById("t");
		if (!_el) throw new Error("missing element");
		const root = _el;
		const r = extractTable(root);
		expect(r.columns).toEqual(["col1", "col2"]);
		expect(r.rowCount).toBe(2);
		expect(r.rows).toEqual([
			{ col1: "Name", col2: "Age" },
			{ col1: "Carol", col2: "40" },
		]);
	});

	it("names unnamed columns colN", () => {
		document.body.innerHTML = `
			<table id="t">
				<tr><td></td><td></td></tr>
				<tr><td>x</td><td>y</td></tr>
			</table>`;
		const _el = document.getElementById("t");
		if (!_el) throw new Error("missing element");
		const root = _el;
		const r = extractTable(root);
		expect(r.columns).toEqual(["col1", "col2"]);
	});

	it("returns empty when there is no table", () => {
		document.body.innerHTML = `<div id="d">no table here</div>`;
		const _el = document.getElementById("d");
		if (!_el) throw new Error("missing element");
		const root = _el;
		const r = extractTable(root);
		expect(r.columns).toEqual([]);
		expect(r.rows).toEqual([]);
		expect(r.rowCount).toBe(0);
	});

	it("reads a table nested inside the matched container", () => {
		document.body.innerHTML = `
			<div id="wrap"><table><thead><tr><th>A</th></tr></thead><tbody><tr><td>1</td></tr></tbody></table></div>`;
		const _el = document.getElementById("wrap");
		if (!_el) throw new Error("missing element");
		const root = _el;
		const r = extractTable(root);
		expect(r.columns).toEqual(["A"]);
		expect(r.rows).toEqual([{ A: "1" }]);
	});
});

describe("extractList", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("extracts text from each li", () => {
		document.body.innerHTML = `<ul id="l"><li>one</li><li>two</li></ul>`;
		const _el = document.getElementById("l");
		if (!_el) throw new Error("missing element");
		const root = _el;
		const r = extractList(root);
		expect(r.count).toBe(2);
		expect(r.items).toEqual([{ text: "one" }, { text: "two" }]);
	});

	it("extracts named fields via relative selectors", () => {
		document.body.innerHTML = `
			<div id="l">
				<div class="row"><span class="name">Alice</span><span class="age">30</span></div>
				<div class="row"><span class="name">Bob</span><span class="age">25</span></div>
			</div>`;
		const _el = document.getElementById("l");
		if (!_el) throw new Error("missing element");
		const root = _el;
		const r = extractList(root, { name: ".name", age: ".age" });
		expect(r.count).toBe(2);
		expect(r.items).toEqual([
			{ name: "Alice", age: "30" },
			{ name: "Bob", age: "25" },
		]);
	});

	it("returns an empty list for an empty container", () => {
		document.body.innerHTML = `<div id="l"></div>`;
		const _el = document.getElementById("l");
		if (!_el) throw new Error("missing element");
		const root = _el;
		const r = extractList(root);
		expect(r.count).toBe(0);
		expect(r.items).toEqual([]);
	});
});

describe("extractListFrom", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("treats every matched element as an item", () => {
		document.body.innerHTML = `<ul id="l"><li>one</li><li>two</li><li>three</li></ul>`;
		const matches = Array.from(document.querySelectorAll("#l li"));
		const r = extractListFrom(matches);
		expect(r.count).toBe(3);
		expect(r.items).toEqual([
			{ text: "one" },
			{ text: "two" },
			{ text: "three" },
		]);
	});

	it("reads named fields from every matched element", () => {
		document.body.innerHTML = `
			<div class="row"><span class="n">X</span><span class="v">1</span></div>
			<div class="row"><span class="n">Y</span><span class="v">2</span></div>`;
		const matches = Array.from(document.querySelectorAll(".row"));
		const r = extractListFrom(matches, { name: ".n", val: ".v" });
		expect(r.count).toBe(2);
		expect(r.items).toEqual([
			{ name: "X", val: "1" },
			{ name: "Y", val: "2" },
		]);
	});

	it("keeps container behaviour for a single match", () => {
		document.body.innerHTML = `<ul id="l"><li>one</li><li>two</li></ul>`;
		const matches = Array.from(document.querySelectorAll("#l"));
		const r = extractListFrom(matches);
		expect(r.count).toBe(2);
		expect(r.items).toEqual([{ text: "one" }, { text: "two" }]);
	});

	it("returns nothing for no matches", () => {
		const r = extractListFrom([]);
		expect(r.count).toBe(0);
		expect(r.items).toEqual([]);
	});
});

describe("extractListFrom — alternative field selectors", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("falls back to the second form when the first is absent", () => {
		document.body.innerHTML = `
			<div class="row"><span class="yP">Alice</span></div>
			<div class="row"><span class="zF">Bob</span></div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		const r = extractListFrom(rows, { sender: "span.yP||span.zF" });
		expect(r.items).toEqual([{ sender: "Alice" }, { sender: "Bob" }]);
	});

	it("prefers the first form when both exist", () => {
		document.body.innerHTML = `
			<div class="row"><span class="yP">Primary</span><span class="zF">Secondary</span></div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		const r = extractListFrom(rows, { sender: "span.yP||span.zF" });
		expect(r.items).toEqual([{ sender: "Primary" }]);
	});

	it("treats an EMPTY first match as absent and keeps looking", () => {
		document.body.innerHTML = `
			<div class="row"><span class="yP"></span><span class="zF">Fallback</span></div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		const r = extractListFrom(rows, { sender: "span.yP||span.zF" });
		expect(r.items).toEqual([{ sender: "Fallback" }]);
	});

	it("supports alternatives in the attribute form", () => {
		document.body.innerHTML = `
			<div class="row"><span class="yP" email="a@example.test">Alice</span></div>
			<div class="row"><span class="zF" data-email="b@example.test">Bob</span></div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		const r = extractListFrom(rows, {
			email: "@span.yP@email||@span.zF@data-email",
		});
		expect(r.items).toEqual([
			{ email: "a@example.test" },
			{ email: "b@example.test" },
		]);
	});

	it("does not split a single pipe used by the CSS |= operator", () => {
		document.body.innerHTML = `
			<div class="row"><span lang="en-GB"><b class="val">7</b></span></div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		const r = extractListFrom(rows, { v: "[lang|=en] .val" });
		expect(r.items).toEqual([{ v: "7" }]);
	});

	it("does not split || inside a quoted attribute value", () => {
		document.body.innerHTML = `
			<div class="row"><span title="a||b"><b class="val">X</b></span></div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		const r = extractListFrom(rows, { t: '[title="a||b"] .val' });
		expect(r.items).toEqual([{ t: "X" }]);
	});

	it("does not end a quoted value at an escaped quote before ||", () => {
		// Two alternatives exactly when the escaped quote keeps the value open:
		// the first matches nothing, the second supplies "X". Splitting inside the
		// quote would leave a malformed first selector and throw instead.
		document.body.innerHTML = `
			<div class="row"><b class="val">X</b></div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		const r = extractListFrom(rows, { t: '[data-x="a\\"||b"] .val||.val' });
		expect(r.items).toEqual([{ t: "X" }]);
	});

	it("rejects a field whose selector is only separators", () => {
		document.body.innerHTML = `<div class="row"><span>A</span></div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		expect(() => extractListFrom(rows, { x: "||" })).toThrow(/field "x"/);
	});

	it("rejects a malformed field selector even when no items match", () => {
		document.body.innerHTML = `<div class="row"><span>A</span></div>`;
		expect(() => extractListFrom([], { x: "||" })).toThrow(/field "x"/);
	});
});

describe("extractListFrom — visibleOnly", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	function stubCheckVisibility(el: Element, rendered: boolean): void {
		Object.defineProperty(el, "checkVisibility", {
			value: () => rendered,
			configurable: true,
		});
	}

	it("drops items that checkVisibility reports as hidden", () => {
		document.body.innerHTML = `
			<div class="row">one</div>
			<div class="row">two</div>
			<div class="row">three</div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		stubCheckVisibility(rows[0], true);
		stubCheckVisibility(rows[1], false);
		stubCheckVisibility(rows[2], true);
		const r = extractListFrom(rows, undefined, { visibleOnly: true });
		expect(r.items).toEqual([{ text: "one" }, { text: "three" }]);
		expect(r.count).toBe(2);
	});

	it("keeps every item when visibleOnly is off", () => {
		document.body.innerHTML = `
			<div class="row">one</div>
			<div class="row">two</div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		stubCheckVisibility(rows[0], true);
		stubCheckVisibility(rows[1], false);
		const r = extractListFrom(rows);
		expect(r.count).toBe(2);
	});

	it("refuses to guess when the host has no checkVisibility", () => {
		document.body.innerHTML = `<div class="row">one</div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		Object.defineProperty(rows[0], "checkVisibility", {
			value: undefined,
			configurable: true,
		});
		// Fail loud, not open: returning the row would report a filtered list
		// that was never filtered.
		expect(() =>
			extractListFrom(rows, undefined, { visibleOnly: true }),
		).toThrow(/visibleOnly requires Element.checkVisibility/);
	});
});

describe("extractListFrom — attribute fields", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("reads an attribute when the field selector is prefixed with @", () => {
		document.body.innerHTML = `
			<li class="task" data-title="Send report" data-due="2026-10-22">
				<span class="title">Send report</span>
			</li>
			<li class="task" data-title="Book room" data-due="2026-10-23">
				<span class="title">Book room</span>
			</li>`;
		const matches = Array.from(document.querySelectorAll(".task"));
		const r = extractListFrom(matches, {
			title: "@data-title",
			due: "@data-due",
		});
		expect(r.items).toEqual([
			{ title: "Send report", due: "2026-10-22" },
			{ title: "Book room", due: "2026-10-23" },
		]);
	});

	it("reads an attribute from a DESCENDANT with the @ prefix", () => {
		document.body.innerHTML = `
			<li class="task"><a class="link" href="/msg/42">Open</a></li>
			<li class="task"><a class="link" href="/msg/43">Open</a></li>`;
		const matches = Array.from(document.querySelectorAll(".task"));
		const r = extractListFrom(matches, { href: "@.link@href" });
		expect(r.items).toEqual([{ href: "/msg/42" }, { href: "/msg/43" }]);
	});

	it("reads an attribute of a CONTAINER's item (single match, container form)", () => {
		document.body.innerHTML = `
			<ul id="l">
				<li class="task" data-due="2026-10-23"><span class="title">Book room</span></li>
			</ul>`;
		const matches = Array.from(document.querySelectorAll("#l"));
		const r = extractListFrom(matches, { due: "@data-due" });
		expect(r.items).toEqual([{ due: "2026-10-23" }]);
	});

	it("matches the ITEM ITSELF when the field selector matches it", () => {
		document.body.innerHTML = `
			<li class="task" data-due="2026-10-23"><span class="title">Book room</span></li>
			<li class="task" data-due="2026-10-24"><span class="title">Other</span></li>`;
		const matches = Array.from(document.querySelectorAll(".task"));
		const r = extractListFrom(matches, { due: "[data-due]" });
		expect(r.items).toEqual([{ due: "2026-10-23" }, { due: "2026-10-24" }]);
	});

	it("matches the item itself before falling back to a descendant", () => {
		// The item carries data-title AND a descendant with the same class.
		document.body.innerHTML = `
			<li class="task" data-title="from-attribute"><span class="title">from-text</span></li>
			<li class="task" data-title="from-attribute-2"><span class="title">from-text-2</span></li>`;
		const matches = Array.from(document.querySelectorAll(".task"));
		const r = extractListFrom(matches, { title: ".title" });
		expect(r.items).toEqual([{ title: "from-text" }, { title: "from-text-2" }]);
	});

	it("returns an empty string for a missing attribute", () => {
		document.body.innerHTML = `
			<li class="task" data-due=""></li>
			<li class="task" data-due=""></li>`;
		const matches = Array.from(document.querySelectorAll(".task"));
		const r = extractListFrom(matches, { due: "@data-due" });
		expect(r.items).toEqual([{ due: "" }, { due: "" }]);
	});

	it("keeps plain text fields working alongside attribute fields", () => {
		document.body.innerHTML = `
			<li class="task" data-due="2026-10-22"><span class="title">Send report</span></li>
			<li class="task" data-due="2026-10-23"><span class="title">Book room</span></li>`;
		const matches = Array.from(document.querySelectorAll(".task"));
		const r = extractListFrom(matches, {
			title: ".title",
			due: "@data-due",
		});
		expect(r.items).toEqual([
			{ title: "Send report", due: "2026-10-22" },
			{ title: "Book room", due: "2026-10-23" },
		]);
	});
});

// ── Regression: F5 (table header/duplicate columns), F4 (single match),
//    F14 (invalid field selector must name the field) ─────────────────
describe("extractTable header and duplicate columns", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("does not emit the header row as data when thead has no tbody", () => {
		document.body.innerHTML = `
			<table id="t">
				<thead><tr><th>Name</th><th>Age</th></tr></thead>
				<tr><td>Alice</td><td>30</td></tr>
			</table>`;
		const root = document.getElementById("t");
		if (!root) throw new Error("missing table");
		const r = extractTable(root);
		expect(r.columns).toEqual(["Name", "Age"]);
		expect(r.rows).toEqual([{ Name: "Alice", Age: "30" }]);
		expect(r.rowCount).toBe(1);
	});

	it("keeps both values when two columns share a name", () => {
		document.body.innerHTML = `
			<table id="t">
				<tr><th>Amount</th><th>Amount</th></tr>
				<tr><td>100</td><td>200</td></tr>
			</table>`;
		const root = document.getElementById("t");
		if (!root) throw new Error("missing table");
		const r = extractTable(root);
		expect(r.columns).toEqual(["Amount", "Amount_2"]);
		expect(Object.values(r.rows[0])).toEqual(["100", "200"]);
	});

	it("treats a first row of th cells as the header even without a thead", () => {
		// <th> is the explicit HTML signal for a header cell, so a tbody whose
		// first row is all <th> keeps its header semantics.
		document.body.innerHTML = `
			<table id="t">
				<tbody>
					<tr><th>Name</th><th>Age</th></tr>
					<tr><td>Carol</td><td>40</td></tr>
				</tbody>
			</table>`;
		const root = document.getElementById("t");
		if (!root) throw new Error("missing table");
		const r = extractTable(root);
		expect(r.columns).toEqual(["Name", "Age"]);
		expect(r.rowCount).toBe(1);
		expect(r.rows).toEqual([{ Name: "Carol", Age: "40" }]);
	});

	it("keeps every row of a td-only grid (a spreadsheet has no header row)", () => {
		// The Sheets fixture shape: a grid of contenteditable cells, no thead,
		// no th. The first row's value must survive as DATA, not become a
		// column name. Regression guard for benchmark scenario S06.
		document.body.innerHTML = `
			<table id="grid">
				<tbody>
					<tr><td><div contenteditable="true" data-col="A" data-row="1">Widget</div></td><td><div contenteditable="true" data-col="B" data-row="1"></div></td></tr>
					<tr><td><div contenteditable="true" data-col="A" data-row="2"></div></td><td><div contenteditable="true" data-col="B" data-row="2"></div></td></tr>
					<tr><td><div contenteditable="true" data-col="A" data-row="3"></div></td><td><div contenteditable="true" data-col="B" data-row="3"></div></td></tr>
				</tbody>
			</table>`;
		const root = document.getElementById("grid");
		if (!root) throw new Error("missing table");
		const r = extractTable(root);
		expect(r.columns).toEqual(["col1", "col2"]);
		expect(r.rowCount).toBe(3);
		expect(r.rows[0]).toEqual({ col1: "Widget", col2: "" });
	});

	it("returns no columns and no rows for an empty table", () => {
		document.body.innerHTML = `<table id="t"><tbody></tbody></table>`;
		const root = document.getElementById("t");
		if (!root) throw new Error("missing table");
		const r = extractTable(root);
		expect(r.columns).toEqual([]);
		expect(r.rows).toEqual([]);
		expect(r.rowCount).toBe(0);
	});

	it("returns no columns and no rows for a table whose only row is empty", () => {
		document.body.innerHTML = `<table id="t"><tbody><tr></tr></tbody></table>`;
		const root = document.getElementById("t");
		if (!root) throw new Error("missing table");
		const r = extractTable(root);
		expect(r.columns).toEqual([]);
		expect(r.rows).toEqual([]);
		expect(r.rowCount).toBe(0);
	});
});

describe("extractListFrom single-match disambiguation", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("treats one matched ITEM as one item, not as a container", () => {
		document.body.innerHTML = `
			<div class="row"><span class="name">Alice</span><span class="val">30</span></div>`;
		const matches = Array.from(document.querySelectorAll(".row"));
		const r = extractListFrom(matches, { name: ".name", val: ".val" });
		expect(r.count).toBe(1);
		expect(r.items).toEqual([{ name: "Alice", val: "30" }]);
	});

	it("still expands a single semantic list container into its rows", () => {
		document.body.innerHTML = `<ul id="l"><li>one</li><li>two</li></ul>`;
		const matches = Array.from(document.querySelectorAll("#l"));
		const r = extractListFrom(matches);
		expect(r.count).toBe(2);
	});

	it("honours an explicit container flag for a non-semantic wrapper", () => {
		document.body.innerHTML = `<div id="wrap"><div class="x">a</div><div class="x">b</div></div>`;
		const matches = Array.from(document.querySelectorAll("#wrap"));
		const r = extractListFrom(matches, undefined, { container: true });
		expect(r.count).toBe(2);
	});
});

describe("extractListFrom invalid field selector", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("preserves the original error as the cause", () => {
		document.body.innerHTML = `<div class="row"><span class="name">Alice</span></div>`;
		const matches = Array.from(document.querySelectorAll(".row"));
		let caught: unknown;
		try {
			extractListFrom(matches, { bad: "[" });
		} catch (err) {
			caught = err;
		}
		expect(caught).toBeInstanceOf(Error);
		expect((caught as Error).cause).toBeInstanceOf(Error);
	});

	it("names the offending field instead of failing with a bare DOMException", () => {
		document.body.innerHTML = `
			<div class="row"><span class="name">Alice</span><span class="val">30</span></div>`;
		const matches = Array.from(document.querySelectorAll(".row"));
		expect(() => extractListFrom(matches, { good: ".name", bad: "[" })).toThrow(
			/bad/,
		);
	});
});

describe("extractList at the executor level", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("passes EVERY match to the extractor, not only the first", async () => {
		// A one-character change in handleExtractList (findAllElements(target)[0])
		// would silently collapse multi-row extraction to one row.
		document.body.innerHTML = `
			<div class="row"><span class="name">Alice</span></div>
			<div class="row"><span class="name">Bob</span></div>`;
		const { executeCommand } = await import("./commandExecutor");
		const res = (await executeCommand({
			id: "1",
			action: "extractList",
			target: { selector: ".row" },
			options: { fields: { name: ".name" } },
		})) as { data?: { count?: number; items?: unknown[] } };
		expect(res.data?.count).toBe(2);
		expect(res.data?.items).toEqual([{ name: "Alice" }, { name: "Bob" }]);
	});
});
