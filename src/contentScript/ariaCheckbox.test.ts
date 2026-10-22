import { beforeEach, describe, expect, it } from "bun:test";
import "../test/domSetup";

/**
 * An ARIA checkbox is a DIV with role="checkbox" and aria-checked, not an
 * <input type=checkbox>. Google Tasks, Material UI and most modern design
 * systems render their checkboxes this way.
 *
 * `check`/`uncheck` must drive aria-checked, and must be IDEMPOTENT: the
 * state must end up as asked, whether it was already there or not.
 */
/** An app handler that flips aria-checked on every click, like a real box. */
function toggleOnClick(el: Element): void {
	el.addEventListener("click", () => {
		el.setAttribute(
			"aria-checked",
			el.getAttribute("aria-checked") === "true" ? "false" : "true",
		);
	});
}

describe("ARIA checkbox (role=checkbox) support", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("checks an aria-checkbox that is not checked yet", async () => {
		document.body.innerHTML = `<div id="cb" role="checkbox" aria-checked="false"></div>`;
		const el = document.getElementById("cb");
		if (!el) throw new Error("missing element");
		toggleOnClick(el);

		const { executeCommand } = await import("./commandExecutor");
		await executeCommand({
			id: "1",
			action: "check",
			target: { selector: "#cb" },
		});

		expect(el.getAttribute("aria-checked")).toBe("true");
	});

	it("unchecks an aria-checkbox that is checked", async () => {
		document.body.innerHTML = `<div id="cb" role="checkbox" aria-checked="true"></div>`;
		const el = document.getElementById("cb");
		if (!el) throw new Error("missing element");
		toggleOnClick(el);

		const { executeCommand } = await import("./commandExecutor");
		await executeCommand({
			id: "1",
			action: "uncheck",
			target: { selector: "#cb" },
		});

		expect(el.getAttribute("aria-checked")).toBe("false");
	});

	it("is idempotent: checking an already-checked box leaves it checked", async () => {
		document.body.innerHTML = `<div id="cb" role="checkbox" aria-checked="true"></div>`;
		const el = document.getElementById("cb");
		if (!el) throw new Error("missing element");

		const { executeCommand } = await import("./commandExecutor");
		await executeCommand({
			id: "1",
			action: "check",
			target: { selector: "#cb" },
		});

		expect(el.getAttribute("aria-checked")).toBe("true");
	});

	it("dispatches a click so the app's own handler runs", async () => {
		document.body.innerHTML = `<div id="cb" role="checkbox" aria-checked="false"></div>`;
		const el = document.getElementById("cb");
		if (!el) throw new Error("missing element");

		let clicks = 0;
		el.addEventListener("click", () => {
			clicks++;
			el.setAttribute(
				"aria-checked",
				el.getAttribute("aria-checked") === "true" ? "false" : "true",
			);
		});

		const { executeCommand } = await import("./commandExecutor");
		const res = await executeCommand({
			id: "1",
			action: "check",
			target: { selector: "#cb" },
		});

		expect(res.success).toBe(true);
		expect(clicks).toBe(1);
		// The app's own handler is the authority: it toggled the state.
		expect(el.getAttribute("aria-checked")).toBe("true");
	});

	it("still handles a native input checkbox", async () => {
		document.body.innerHTML = `<input id="cb" type="checkbox">`;
		const el = document.getElementById("cb") as HTMLInputElement;
		if (!el) throw new Error("missing element");

		const { executeCommand } = await import("./commandExecutor");
		await executeCommand({
			id: "1",
			action: "check",
			target: { selector: "#cb" },
		});

		expect(el.checked).toBe(true);
	});

	it("rejects an element that is not a checkbox at all", async () => {
		document.body.innerHTML = `<div id="nope">not a checkbox</div>`;

		const { executeCommand } = await import("./commandExecutor");
		const res = await executeCommand({
			id: "1",
			action: "check",
			target: { selector: "#nope" },
		});

		expect(res.success).toBe(false);
		expect(res.error).toMatch(/checkbox/i);
	});
});

describe("ARIA checkbox tri-state (aria-checked=mixed)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("unchecks a Mixed checkbox instead of silently doing nothing", async () => {
		document.body.innerHTML = `<div id="cb" role="checkbox" aria-checked="mixed"></div>`;
		const el = document.getElementById("cb");
		if (!el) throw new Error("missing element");
		let clicks = 0;
		el.addEventListener("click", () => {
			clicks++;
			// The app clears the mixed state on its first click.
			el.setAttribute("aria-checked", "false");
		});

		const { executeCommand } = await import("./commandExecutor");
		await executeCommand({
			id: "1",
			action: "uncheck",
			target: { selector: "#cb" },
		});

		expect(el.getAttribute("aria-checked")).toBe("false");
		expect(clicks).toBe(1);
	});

	it("clicks a second time when the first click lands on the wrong state", async () => {
		document.body.innerHTML = `<div id="cb" role="checkbox" aria-checked="mixed"></div>`;
		const el = document.getElementById("cb");
		if (!el) throw new Error("missing element");
		// Cycle: mixed -> true -> false -> true ...
		let clicks = 0;
		el.addEventListener("click", () => {
			clicks++;
			const next =
				el.getAttribute("aria-checked") === "mixed"
					? "true"
					: el.getAttribute("aria-checked") === "true"
						? "false"
						: "true";
			el.setAttribute("aria-checked", next);
		});

		const { executeCommand } = await import("./commandExecutor");
		const res = await executeCommand({
			id: "1",
			action: "uncheck",
			target: { selector: "#cb" },
		});

		expect(res.success).toBe(true);
		expect(clicks).toBe(2);
		expect(el.getAttribute("aria-checked")).toBe("false");
	});

	it("throws with the expected and actual state when the box ignores clicks", async () => {
		document.body.innerHTML = `<div id="cb" role="checkbox" aria-checked="false"></div>`;
		const el = document.getElementById("cb");
		if (!el) throw new Error("missing element");
		let clicks = 0;
		el.addEventListener("click", () => {
			clicks++;
		});

		const { executeCommand } = await import("./commandExecutor");
		const res = await executeCommand({
			id: "1",
			action: "check",
			target: { selector: "#cb" },
		});

		expect(res.success).toBe(false);
		expect(res.error).toContain('aria-checked="true"');
		expect(res.error).toContain('still "false"');
		expect(clicks).toBe(2);
		// The attribute is not force-written to the requested value.
		expect(el.getAttribute("aria-checked")).toBe("false");
	});

	it("does not click an already-correct checkbox (no double toggle)", async () => {
		document.body.innerHTML = `<div id="cb" role="checkbox" aria-checked="true"></div>`;
		const el = document.getElementById("cb");
		if (!el) throw new Error("missing element");
		let clicks = 0;
		el.addEventListener("click", () => {
			clicks++;
			el.setAttribute(
				"aria-checked",
				el.getAttribute("aria-checked") === "true" ? "false" : "true",
			);
		});

		const { executeCommand } = await import("./commandExecutor");
		await executeCommand({
			id: "1",
			action: "check",
			target: { selector: "#cb" },
		});

		expect(el.getAttribute("aria-checked")).toBe("true");
		expect(clicks).toBe(0);
	});
});
