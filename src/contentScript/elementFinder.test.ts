import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import "../test/domSetup";
import type { TargetSelector } from "../types/commands";
import {
	findElement,
	waitForActionableElement,
	waitForScrollableActionableElement,
} from "./elementFinder";

describe("elementFinder (DOM)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	it("locates an element by CSS selector", () => {
		const el = document.createElement("button");
		el.id = "submit-btn";
		el.textContent = "Submit";
		document.body.appendChild(el);

		const target: TargetSelector = { selector: "#submit-btn" };
		const found = findElement(target);
		expect(found).not.toBeNull();
		expect((found as HTMLElement).id).toBe("submit-btn");
	});
});

describe("waitForActionableElement", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	it("resolves when the element appears shortly after the call", async () => {
		const target: TargetSelector = { selector: "#late" };
		setTimeout(() => {
			const el = document.createElement("button");
			el.id = "late";
			document.body.appendChild(el);
		}, 100);

		const el = await waitForActionableElement(target, { timeoutMs: 2000 });
		expect((el as HTMLElement).id).toBe("late");
	});

	it("rejects with 'not found' wording when the selector never matches", async () => {
		const target: TargetSelector = { selector: "#nope" };
		await expect(
			waitForActionableElement(target, { timeoutMs: 150 }),
		).rejects.toThrow(/not found/i);
	});

	it("rejects with 'not visible' wording when present but hidden", async () => {
		const el = document.createElement("button");
		el.id = "hidden";
		el.style.display = "none";
		document.body.appendChild(el);
		const target: TargetSelector = { selector: "#hidden" };
		await expect(
			waitForActionableElement(target, { timeoutMs: 150 }),
		).rejects.toThrow(/not visible/i);
	});

	it("rejects with 'disabled' wording when disabled and requireEnabled is set", async () => {
		const el = document.createElement("button");
		el.id = "off";
		el.disabled = true;
		document.body.appendChild(el);
		const target: TargetSelector = { selector: "#off" };
		await expect(
			waitForActionableElement(target, {
				timeoutMs: 150,
				requireEnabled: true,
			}),
		).rejects.toThrow(/disabled/i);
	});

	it("resolves for a disabled element when requireEnabled is false", async () => {
		const el = document.createElement("button");
		el.id = "off2";
		el.disabled = true;
		document.body.appendChild(el);
		const target: TargetSelector = { selector: "#off2" };
		const found = await waitForActionableElement(target, {
			timeoutMs: 150,
			requireEnabled: false,
		});
		expect((found as HTMLElement).id).toBe("off2");
	});

	it("resolves when the element becomes visible shortly after the call", async () => {
		const el = document.createElement("button");
		el.id = "reveal";
		el.style.display = "none";
		document.body.appendChild(el);
		const target: TargetSelector = { selector: "#reveal" };
		setTimeout(() => {
			el.style.display = "block";
		}, 100);

		const found = await waitForActionableElement(target, { timeoutMs: 2000 });
		expect((found as HTMLElement).id).toBe("reveal");
	});

	it("scrolls a rendered-but-offscreen element into view instead of calling it not visible", async () => {
		const el = document.createElement("button");
		el.id = "below-fold";
		document.body.appendChild(el);
		let scrolled = false;
		const offscreen = {
			x: 0,
			y: 5000,
			width: 100,
			height: 50,
			top: 5000,
			bottom: 5050,
			left: 0,
			right: 100,
			toJSON() {},
		};
		const onscreen = {
			x: 0,
			y: 100,
			width: 100,
			height: 50,
			top: 100,
			bottom: 150,
			left: 0,
			right: 100,
			toJSON() {},
		};
		el.getBoundingClientRect = () =>
			(scrolled ? onscreen : offscreen) as DOMRect;
		el.scrollIntoView = () => {
			scrolled = true;
		};
		const found = await waitForScrollableActionableElement(
			{ selector: "#below-fold" },
			{ timeoutMs: 500 },
		);
		expect(scrolled).toBe(true);
		expect((found as HTMLElement).id).toBe("below-fold");
	});

	it("does not scroll a rendered-but-offscreen element for the default wait (focus, highlight)", async () => {
		const el = document.createElement("button");
		el.id = "below-fold-hover";
		document.body.appendChild(el);
		let scrolled = false;
		const offscreen = {
			x: 0,
			y: 5000,
			width: 100,
			height: 50,
			top: 5000,
			bottom: 5050,
			left: 0,
			right: 100,
			toJSON() {},
		};
		el.getBoundingClientRect = () => offscreen as DOMRect;
		el.scrollIntoView = () => {
			scrolled = true;
		};
		await expect(
			waitForActionableElement(
				{ selector: "#below-fold-hover" },
				{ timeoutMs: 150 },
			),
		).rejects.toThrow(/not visible/i);
		expect(scrolled).toBe(false);
	});

	it("scrolls an offscreen element at most once per wait, not on every poll", async () => {
		const el = document.createElement("button");
		el.id = "stuck";
		document.body.appendChild(el);
		let scrolls = 0;
		const offscreen = {
			x: 0,
			y: 5000,
			width: 100,
			height: 50,
			top: 5000,
			bottom: 5050,
			left: 0,
			right: 100,
			toJSON() {},
		};
		// Never becomes visible: scrollIntoView is a no-op here, so without the
		// once-per-wait guard the 50ms poll would keep re-scrolling until the
		// deadline (the bug: check() scrolled on every poll).
		el.getBoundingClientRect = () => offscreen as DOMRect;
		el.scrollIntoView = () => {
			scrolls++;
		};
		await expect(
			waitForScrollableActionableElement(
				{ selector: "#stuck" },
				{ timeoutMs: 300 },
			),
		).rejects.toThrow(/not visible/i);
		expect(scrolls).toBe(1);
	});

	it("does not scroll a display:none element (scrolling cannot reveal it)", async () => {
		const el = document.createElement("button");
		el.id = "collapsed";
		el.style.display = "none";
		document.body.appendChild(el);
		let scrolled = false;
		el.scrollIntoView = () => {
			scrolled = true;
		};
		await expect(
			waitForActionableElement({ selector: "#collapsed" }, { timeoutMs: 150 }),
		).rejects.toThrow(/not visible/i);
		expect(scrolled).toBe(false);
	});
});
