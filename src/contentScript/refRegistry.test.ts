import { beforeEach, describe, expect, it } from "bun:test";
import "../test/domSetup";
import { assignRef, clearRefs, refCount, resolveRef } from "./refRegistry";

describe("refRegistry", () => {
	beforeEach(() => {
		clearRefs();
		document.body.innerHTML = "";
	});

	it("mints increasing @eN ids and resolves them back to the element", () => {
		const a = document.createElement("button");
		const b = document.createElement("input");
		document.body.append(a, b);

		const refA = assignRef(a);
		const refB = assignRef(b);
		expect(refA).toBe("@e1");
		expect(refB).toBe("@e2");
		expect(resolveRef(refA)).toBe(a);
		expect(resolveRef(refB)).toBe(b);
	});

	it("returns the same ref id for the same element (no duplicate handles)", () => {
		const a = document.createElement("button");
		document.body.appendChild(a);
		expect(assignRef(a)).toBe(assignRef(a));
	});

	it("throws a stale-ref error for a detached element", () => {
		const a = document.createElement("button");
		document.body.appendChild(a);
		const ref = assignRef(a);

		a.remove();
		expect(() => resolveRef(ref)).toThrow("stale ref");
	});

	it("throws for an unknown ref id", () => {
		expect(() => resolveRef("@e999")).toThrow("stale ref");
	});

	it("clearRefs resets all state and refCount is accurate", () => {
		const a = document.createElement("div");
		const b = document.createElement("span");
		document.body.append(a, b);
		assignRef(a);
		assignRef(b);
		expect(refCount()).toBe(2);
		clearRefs();
		expect(refCount()).toBe(0);
		// After clear, minting starts over at @e1.
		const c = document.createElement("p");
		document.body.appendChild(c);
		expect(assignRef(c)).toBe("@e1");
	});
});
