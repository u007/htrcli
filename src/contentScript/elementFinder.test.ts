import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import "../test/domSetup";
import type { TargetSelector } from "../types/commands";
import { findElement, waitForActionableElement } from "./elementFinder";

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
});
