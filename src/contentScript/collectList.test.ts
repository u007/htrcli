import { beforeEach, describe, expect, it } from "bun:test";
import "../test/domSetup";
import { collectAll, findScrollContainer } from "./collectList";

/**
 * happy-dom has no layout, so scrollHeight/clientHeight are 0. These tests
 * stub them per element to simulate a scrollable virtualized list.
 */
function makeScrollable(
	el: Element,
	scrollHeight: number,
	clientHeight: number,
) {
	Object.defineProperty(el, "scrollHeight", {
		value: scrollHeight,
		configurable: true,
	});
	Object.defineProperty(el, "clientHeight", {
		value: clientHeight,
		configurable: true,
	});
	let top = 0;
	Object.defineProperty(el, "scrollTop", {
		get: () => top,
		set: (v: number) => {
			top = Math.max(0, Math.min(v, scrollHeight - clientHeight));
		},
		configurable: true,
	});
}

describe("collectAll (virtualized list)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("returns the current DOM rows when no scrollable container exists", async () => {
		document.body.innerHTML = `<div class="row">a</div><div class="row">b</div>`;
		const result = await collectAll({ selector: ".row" });
		expect(result.elements.map((e) => e.text)).toEqual(["a", "b"]);
		expect(result.scrolled).toBe(false);
		expect(result.steps).toBe(0);
	});

	it("finds the scroll container from the first match's ancestors", () => {
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto"><div><div class="row">a</div></div></div>`;
		const scroller = document.getElementById("scroller");
		if (!scroller) throw new Error("missing scroller");
		makeScrollable(scroller, 2000, 500);
		const found = findScrollContainer(
			document.querySelector(".row") ?? undefined,
		);
		expect(found).toBe(scroller);
	});

	it("scrolls the container and accumulates rows that were not in the DOM initially", async () => {
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto"><div id="rows"></div></div>`;
		const scroller = document.getElementById("scroller");
		const rows = document.getElementById("rows");
		if (!scroller || !rows) throw new Error("missing fixtures");

		// Virtualized: only 2 rows exist at a time, and the DOM content is
		// replaced as the list scrolls. The scrollTop setter re-renders, which
		// is what a real virtualized list does.
		const WINDOW = 2;
		const TOTAL = 10;
		let firstVisible = 0;
		const render = () => {
			rows.innerHTML = "";
			for (let i = 0; i < WINDOW; i++) {
				const index = firstVisible + i;
				if (index >= TOTAL) break;
				const row = document.createElement("div");
				row.className = "row";
				row.setAttribute("data-id", `row-${index}`);
				row.textContent = `row ${index}`;
				rows.appendChild(row);
			}
		};
		let top = 0;
		Object.defineProperty(scroller, "scrollHeight", {
			value: TOTAL * 100,
			configurable: true,
		});
		Object.defineProperty(scroller, "clientHeight", {
			value: 200,
			configurable: true,
		});
		Object.defineProperty(scroller, "scrollTop", {
			get: () => top,
			set: (v: number) => {
				top = Math.max(0, Math.min(v, TOTAL * 100 - 200));
				firstVisible = Math.floor(top / 100);
				render();
			},
			configurable: true,
		});
		render();

		const result = await collectAll(
			{ selector: ".row" },
			{ stepDelayMs: 0, maxSteps: 10 },
		);

		expect(result.scrolled).toBe(true);
		// Rows are de-duped by data-id, so the virtualized window does not
		// cause duplicates.
		const ids = result.elements.map((e) => e.attributes?.["data-id"]);
		expect(new Set(ids).size).toBe(ids.length);
		// Every row was seen exactly once across the scroll.
		expect(result.elements.length).toBe(TOTAL);
	});

	it("de-dupes rows that stay in the DOM across scrolls", async () => {
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto"><div class="row" data-id="a">a</div><div class="row" data-id="b">b</div></div>`;
		const scroller = document.getElementById("scroller");
		if (!scroller) throw new Error("missing scroller");
		makeScrollable(scroller, 2000, 500);
		const result = await collectAll(
			{ selector: ".row" },
			{ stepDelayMs: 0, maxSteps: 3 },
		);
		expect(result.elements.length).toBe(2);
	});

	it("reports keyStrategy 'key' when rows carry a stable id", async () => {
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto"><div class="row" data-id="a">a</div></div>`;
		const scroller = document.getElementById("scroller");
		if (!scroller) throw new Error("missing scroller");
		makeScrollable(scroller, 2000, 500);
		const result = await collectAll(
			{ selector: ".row" },
			{ stepDelayMs: 0, maxSteps: 2 },
		);
		expect(result.keyStrategy).toBe("key");
	});

	it("reports keyStrategy 'text' when rows have no id", async () => {
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto"><div class="row">a</div></div>`;
		const scroller = document.getElementById("scroller");
		if (!scroller) throw new Error("missing scroller");
		makeScrollable(scroller, 2000, 500);
		const result = await collectAll(
			{ selector: ".row" },
			{ stepDelayMs: 0, maxSteps: 2 },
		);
		expect(result.keyStrategy).toBe("text");
	});

	it("reports keyStrategy 'index' when rows have neither id nor text", async () => {
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto"><div class="row"></div></div>`;
		const scroller = document.getElementById("scroller");
		if (!scroller) throw new Error("missing scroller");
		makeScrollable(scroller, 2000, 500);
		const result = await collectAll(
			{ selector: ".row" },
			{ stepDelayMs: 0, maxSteps: 2 },
		);
		expect(result.keyStrategy).toBe("index");
	});

	it("reports truncated when the step limit is reached", async () => {
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto"><div class="row" data-id="a">a</div></div>`;
		const scroller = document.getElementById("scroller");
		if (!scroller) throw new Error("missing scroller");
		makeScrollable(scroller, 100000, 500);
		const result = await collectAll(
			{ selector: ".row" },
			{ stepDelayMs: 0, maxSteps: 2 },
		);
		expect(result.truncated).toBe(true);
		expect(result.steps).toBe(2);
	});
});

// ── Regression: F1 (maxItems) and F13 (scroll restoration) ──────────
describe("collectAll option contracts", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("maxItems caps the returned elements, not only the scroll loop", async () => {
		// 10 rows are already in the DOM, so the initial harvest sees all of
		// them before the scroll loop can break.
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto">${Array.from(
			{ length: 10 },
			(_, i) => `<div class="row" data-id="row-${i}">row ${i}</div>`,
		).join("")}</div>`;
		const scroller = document.getElementById("scroller");
		if (!scroller) throw new Error("missing scroller");
		makeScrollable(scroller, 4000, 500);

		const result = await collectAll(
			{ selector: ".row" },
			{ stepDelayMs: 0, maxItems: 3 },
		);

		expect(result.elements.length).toBe(3);
		expect(result.nodes.length).toBe(3);
	});

	it("restores the container's scroll position after collecting", async () => {
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto">${Array.from(
			{ length: 6 },
			(_, i) => `<div class="row" data-id="row-${i}">row ${i}</div>`,
		).join("")}</div>`;
		const scroller = document.getElementById("scroller");
		if (!scroller) throw new Error("missing scroller");
		makeScrollable(scroller, 4000, 500);
		scroller.scrollTop = 120;

		await collectAll({ selector: ".row" }, { stepDelayMs: 0 });

		expect(scroller.scrollTop).toBe(120);
	});
});

// ── Regression: F11 (refs must be minted while the node is live) ────
describe("collectAll onRow hook", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("fires for each newly collected row before the row is recycled", async () => {
		// A virtualized list that replaces its rows on every scroll, so a row's
		// node is detached by the time the loop ends.
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto"><div id="rows"></div></div>`;
		const scroller = document.getElementById("scroller");
		const rows = document.getElementById("rows");
		if (!scroller || !rows) throw new Error("missing fixtures");
		makeScrollable(scroller, 4000, 500);
		const WINDOW = 2;
		const TOTAL = 8;
		let firstVisible = 0;
		const render = () => {
			rows.innerHTML = "";
			for (let i = 0; i < WINDOW; i++) {
				const index = firstVisible + i;
				if (index >= TOTAL) break;
				const row = document.createElement("div");
				row.className = "row";
				row.setAttribute("data-id", `row-${index}`);
				row.textContent = `row ${index}`;
				rows.appendChild(row);
			}
		};
		render();
		Object.defineProperty(scroller, "scrollTop", {
			configurable: true,
			get: () => firstVisible * 100,
			set: (v: number) => {
				firstVisible = Math.min(Math.floor(v / 100), TOTAL - WINDOW);
				render();
			},
		});

		const liveAtCallTime: boolean[] = [];
		await collectAll(
			{ selector: ".row" },
			{
				stepDelayMs: 0,
				onRow: (el) => {
					liveAtCallTime.push(el.isConnected);
				},
			},
		);

		expect(liveAtCallTime.length).toBeGreaterThan(0);
		expect(liveAtCallTime.every(Boolean)).toBe(true);
	});

	it("harvested nodes detach after scrolling a recycling virtualizer (refs go stale)", async () => {
		// A virtualized list that recycles its nodes on scroll. After collectAll
		// scrolls to the bottom, every node it harvested is detached — a ref minted
		// at harvest time no longer resolves in a follow-up command. Callers must
		// re-harvest or find --ref on the currently-visible row.
		document.body.innerHTML =
			'<div id="scroller" style="overflow-y:auto"><div id="rows"></div></div>';
		const scroller = document.getElementById("scroller");
		const rows = document.getElementById("rows");
		if (!scroller || !rows) throw new Error("missing fixtures");
		makeScrollable(scroller, 4000, 500);
		const WINDOW = 2;
		const TOTAL = 8;
		let firstVisible = 0;
		const render = () => {
			rows.innerHTML = "";
			for (let i = 0; i < WINDOW; i++) {
				const index = firstVisible + i;
				if (index >= TOTAL) break;
				const row = document.createElement("div");
				row.className = "row";
				row.setAttribute("data-id", `row-${index}`);
				row.textContent = `row ${index}`;
				rows.appendChild(row);
			}
		};
		render();
		Object.defineProperty(scroller, "scrollTop", {
			configurable: true,
			get: () => firstVisible * 100,
			set: (v: number) => {
				firstVisible = Math.min(Math.floor(v / 100), TOTAL - WINDOW);
				render();
			},
		});

		const harvested: Element[] = [];
		await collectAll(
			{ selector: ".row" },
			{
				stepDelayMs: 0,
				maxSteps: 10,
				onRow: (el) => harvested.push(el),
			},
		);

		expect(harvested.length).toBeGreaterThan(0);
		// Every harvested node was detached by the scroll loop: refs built from
		// them are stale. A follow-up  would report 'stale ref'.
		expect(harvested.every((el) => !el.isConnected)).toBe(true);
	});
});

// ── Regression: F17 (findScrollContainer must handle a table wrapper) ─
describe("findScrollContainer table-wrapper layout", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("finds the scroller above a table that wraps the rows", () => {
		// Gmail-shaped: the scroller is several levels above the row, with a
		// non-scrollable table/tbody in between.
		document.body.innerHTML = `
			<div id="scroller" style="overflow-y:auto">
				<table><tbody><tr><td>noise</td></tr></table>
			</div>`;
		const scroller = document.getElementById("scroller");
		if (!scroller) throw new Error("missing scroller");
		makeScrollable(scroller, 2000, 500);

		const row = document.createElement("div");
		row.className = "row";
		document.body.innerHTML = `
			<div id="scroller2" style="overflow-y:auto">
				<table><tbody><tr id="row"><td>x</td></tr></tbody></table>
			</div>`;
		const scroller2 = document.getElementById("scroller2");
		const rowEl = document.getElementById("row");
		if (!scroller2 || !rowEl) throw new Error("missing fixtures");
		makeScrollable(scroller2, 2000, 500);

		expect(findScrollContainer(rowEl)).toBe(scroller2);
	});
});

// ── Regression: index-keyed rows must not collide across harvests ──────
describe("collectAll index key strategy", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("collects every row of a virtualized list whose rows have no id or text", async () => {
		// Empty rows force the 'index' strategy. The window shows 2 rows and
		// scrolls by one page (2 rows) per step. A row keeps its DOM node for
		// as long as it stays in the window, as a real virtualizer does.
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto"><div id="rows"></div></div>`;
		const scroller = document.getElementById("scroller");
		const rows = document.getElementById("rows");
		if (!scroller || !rows) throw new Error("missing fixtures");
		const WINDOW = 2;
		const TOTAL = 10;
		const ROW_HEIGHT = 100;
		const nodes = new Map<number, HTMLElement>();
		let firstVisible = 0;
		const render = () => {
			rows.innerHTML = "";
			for (let i = 0; i < WINDOW; i++) {
				const index = firstVisible + i;
				if (index >= TOTAL) break;
				let row = nodes.get(index);
				if (!row) {
					row = document.createElement("div");
					row.className = "row";
					nodes.set(index, row);
				}
				rows.appendChild(row);
			}
		};
		render();
		let top = 0;
		Object.defineProperty(scroller, "scrollHeight", {
			value: TOTAL * ROW_HEIGHT,
			configurable: true,
		});
		Object.defineProperty(scroller, "clientHeight", {
			value: WINDOW * ROW_HEIGHT,
			configurable: true,
		});
		Object.defineProperty(scroller, "scrollTop", {
			get: () => top,
			set: (v: number) => {
				top = Math.max(0, Math.min(v, (TOTAL - WINDOW) * ROW_HEIGHT));
				firstVisible = Math.floor(top / ROW_HEIGHT);
				render();
			},
			configurable: true,
		});

		const result = await collectAll(
			{ selector: ".row" },
			{ stepDelayMs: 0, maxSteps: 50 },
		);

		expect(result.keyStrategy).toBe("index");
		expect(result.elements.length).toBe(TOTAL);
		expect(result.truncated).toBe(false);
	});
});

describe("collectAll scroll restore with onRow", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("leaves the scroll at the end when refs are minted, so their nodes are not recycled", async () => {
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto">${Array.from(
			{ length: 6 },
			(_, i) => `<div class="row" data-id="row-${i}">row ${i}</div>`,
		).join("")}</div>`;
		const scroller = document.getElementById("scroller");
		if (!scroller) throw new Error("missing scroller");
		makeScrollable(scroller, 4000, 500);
		scroller.scrollTop = 120;

		await collectAll({ selector: ".row" }, { stepDelayMs: 0, onRow: () => {} });

		expect(scroller.scrollTop).toBe(3500);
	});
});

describe("collectAll truncation cause", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("reports maxSteps when the step limit cut off a list that was still growing", async () => {
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto"><div class="row" data-id="a">a</div></div>`;
		const scroller = document.getElementById("scroller");
		if (!scroller) throw new Error("missing scroller");
		makeScrollable(scroller, 100000, 500);
		const result = await collectAll(
			{ selector: ".row" },
			{ stepDelayMs: 0, maxSteps: 2 },
		);
		expect(result.truncated).toBe(true);
		expect(result.truncatedBy).toBe("maxSteps");
	});

	it("does not report maxSteps when the list ended on the last allowed step", async () => {
		// The scroll stops at 500 after one step, so step two changes nothing.
		// The list has ended exactly at the step limit, so nothing was cut off.
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto"><div class="row" data-id="a">a</div></div>`;
		const scroller = document.getElementById("scroller");
		if (!scroller) throw new Error("missing scroller");
		makeScrollable(scroller, 1000, 500);
		const result = await collectAll(
			{ selector: ".row" },
			{ stepDelayMs: 0, maxSteps: 2 },
		);
		expect(result.steps).toBe(2);
		expect(result.truncated).toBe(false);
		expect(result.truncatedBy).toBeNull();
	});

	it("reports maxItems, not maxSteps, when the item cap is what stopped collection", async () => {
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto">${Array.from(
			{ length: 10 },
			(_, i) => `<div class="row" data-id="row-${i}">row ${i}</div>`,
		).join("")}</div>`;
		const scroller = document.getElementById("scroller");
		if (!scroller) throw new Error("missing scroller");
		makeScrollable(scroller, 4000, 500);
		const result = await collectAll(
			{ selector: ".row" },
			{ stepDelayMs: 0, maxItems: 3, maxSteps: 2 },
		);
		expect(result.truncatedBy).toBe("maxItems");
	});
});

/**
 * happy-dom ships no `Element.checkVisibility`, so every row under test has to
 * carry a stub. `rendered` is a value or a getter, so a row can change state
 * between harvests.
 */
function setRendered(el: Element, rendered: boolean | (() => boolean)) {
	Object.defineProperty(el, "checkVisibility", {
		value: typeof rendered === "function" ? rendered : () => rendered,
		configurable: true,
	});
}

describe("collectAll — visibleOnly", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("keeps every row when the flag is off", async () => {
		document.body.innerHTML = `<div class="row">a</div><div class="row">b</div><div class="row">c</div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		setRendered(rows[0], true);
		setRendered(rows[1], false);
		setRendered(rows[2], true);
		const result = await collectAll({ selector: ".row" });
		expect(result.elements.map((e) => e.text)).toEqual(["a", "b", "c"]);
	});

	it("drops rows checkVisibility reports hidden", async () => {
		document.body.innerHTML = `<div class="row">a</div><div class="row">b</div><div class="row">c</div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		setRendered(rows[0], true);
		setRendered(rows[1], false);
		setRendered(rows[2], true);
		const result = await collectAll(
			{ selector: ".row" },
			{ visibleOnly: true },
		);
		expect(result.elements.map((e) => e.text)).toEqual(["a", "c"]);
	});

	it("collects a row that is hidden on the first pass and rendered later", async () => {
		// The row must not be marked as seen while it is hidden: doing so would
		// lock it out of the pass that actually renders it.
		document.body.innerHTML = `<div id="scroller" style="overflow-y:auto"><div id="rows"><div class="row" id="r0">a</div><div class="row" id="r1">b</div><div class="row" id="r2">c</div></div></div>`;
		const scroller = document.getElementById("scroller");
		const r0 = document.getElementById("r0");
		const r1 = document.getElementById("r1");
		const r2 = document.getElementById("r2");
		if (!scroller || !r0 || !r1 || !r2) throw new Error("missing fixtures");

		let rowZeroRendered = false;
		setRendered(r0, () => rowZeroRendered);
		setRendered(r1, true);
		setRendered(r2, true);

		let top = 0;
		Object.defineProperty(scroller, "scrollHeight", {
			value: 600,
			configurable: true,
		});
		Object.defineProperty(scroller, "clientHeight", {
			value: 200,
			configurable: true,
		});
		Object.defineProperty(scroller, "scrollTop", {
			get: () => top,
			set: (v: number) => {
				top = Math.max(0, Math.min(v, 400));
				// The scroll re-renders the row into view.
				rowZeroRendered = true;
			},
			configurable: true,
		});

		const result = await collectAll(
			{ selector: ".row" },
			{ visibleOnly: true, stepDelayMs: 0, maxSteps: 5 },
		);
		const texts = result.elements.map((e) => e.text);
		expect(texts).toEqual(["b", "c", "a"]);
		expect(new Set(texts).size).toBe(3);
	});

	it("refuses to guess when the host has no checkVisibility", async () => {
		document.body.innerHTML = `<div class="row">a</div>`;
		const rows = Array.from(document.querySelectorAll(".row"));
		Object.defineProperty(rows[0], "checkVisibility", {
			value: undefined,
			configurable: true,
		});
		await expect(
			collectAll({ selector: ".row" }, { visibleOnly: true }),
		).rejects.toThrow(/visibleOnly requires Element.checkVisibility/);
	});
});
