import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import "../test/domSetup";
import type { Command, CommandAction } from "../types/commands";
import { executeCommand } from "./commandExecutor";

// Test command validation logic (extracted from commandExecutor)
const ACTIONS_REQUIRING_TARGET = new Set([
	"find",
	"findAll",
	"wait",
	"isVisible",
	"isEnabled",
	"getValue",
	"getAttribute",
	"getText",
	"getHTML",
	"getOuterHTML",
	"getBoundingBox",
	"getComputedStyle",
	"xpath",
	"click",
	"dblclick",
	"rightclick",
	"hover",
	"focus",
	"blur",
	"scrollTo",
	"fill",
	"type",
	"clear",
	"select",
	"check",
	"uncheck",
	"pressKey",
	"selectText",
	"highlight",
]);

const ACTIONS_REQUIRING_VALUE = new Set([
	"fill",
	"type",
	"select",
	"pressKey",
	"navigate",
	"evaluate",
]);

function validateCommand(command: Command): { valid: boolean; error?: string } {
	if (ACTIONS_REQUIRING_TARGET.has(command.action) && !command.target) {
		return {
			valid: false,
			error: `Action "${command.action}" requires a "target" selector in the command`,
		};
	}

	if (
		ACTIONS_REQUIRING_VALUE.has(command.action) &&
		command.value === undefined
	) {
		return {
			valid: false,
			error: `Action "${command.action}" requires a "value" in the command`,
		};
	}

	return { valid: true };
}

describe("Command Validation", () => {
	describe("target validation", () => {
		const targetRequiredActions: CommandAction[] = [
			"find",
			"click",
			"dblclick",
			"rightclick",
			"hover",
			"focus",
			"blur",
			"scrollTo",
			"fill",
			"type",
			"clear",
			"select",
			"check",
			"uncheck",
			"pressKey",
			"selectText",
			"highlight",
			"isVisible",
			"isEnabled",
			"getValue",
			"getAttribute",
			"getText",
			"getHTML",
			"getOuterHTML",
			"getBoundingBox",
			"getComputedStyle",
			"xpath",
			"findAll",
			"wait",
		];

		for (const action of targetRequiredActions) {
			it(`should reject "${action}" without target`, () => {
				const result = validateCommand({
					id: "test",
					action,
				});
				expect(result.valid).toBe(false);
				expect(result.error).toContain('requires a "target"');
			});

			it(`should accept "${action}" with target`, () => {
				const needsValue = ACTIONS_REQUIRING_VALUE.has(action);
				const result = validateCommand({
					id: "test",
					action,
					target: { selector: "#test" },
					...(needsValue ? { value: "test-value" } : {}),
				});
				expect(result.valid).toBe(true);
			});
		}
	});

	describe("value validation", () => {
		const valueRequiredActions: CommandAction[] = [
			"fill",
			"type",
			"select",
			"pressKey",
			"navigate",
			"evaluate",
		];

		for (const action of valueRequiredActions) {
			it(`should reject "${action}" without value`, () => {
				const result = validateCommand({
					id: "test",
					action,
					target: { selector: "#test" },
				});
				expect(result.valid).toBe(false);
				expect(result.error).toContain('requires a "value"');
			});

			it(`should accept "${action}" with value`, () => {
				const result = validateCommand({
					id: "test",
					action,
					target: { selector: "#test" },
					value: "test-value",
				});
				expect(result.valid).toBe(true);
			});
		}
	});

	describe("actions without target or value", () => {
		const freeActions: CommandAction[] = [
			"getPageInfo",
			"reload",
			"goBack",
			"goForward",
			"unhighlight",
			"screenshot",
			"listTabs",
			"getTabInfo",
			"switchTab",
		];

		for (const action of freeActions) {
			it(`should accept "${action}" without target or value`, () => {
				const result = validateCommand({
					id: "test",
					action,
				});
				expect(result.valid).toBe(true);
			});
		}
	});

	describe("specific commands", () => {
		it("should validate a complete click command", () => {
			const cmd: Command = {
				id: "cmd-1",
				action: "click",
				target: { selector: "#submit" },
			};
			expect(validateCommand(cmd).valid).toBe(true);
		});

		it("should validate a fill command with value", () => {
			const cmd: Command = {
				id: "cmd-2",
				action: "fill",
				target: { name: "email" },
				value: "test@example.com",
			};
			expect(validateCommand(cmd).valid).toBe(true);
		});

		it("should validate an evaluate command with value", () => {
			const cmd: Command = {
				id: "cmd-3",
				action: "evaluate",
				value: "document.title",
			};
			expect(validateCommand(cmd).valid).toBe(true);
		});

		it("should validate a navigate command with value", () => {
			const cmd: Command = {
				id: "cmd-4",
				action: "navigate",
				value: "https://example.com",
			};
			expect(validateCommand(cmd).valid).toBe(true);
		});

		it("should reject click without target", () => {
			const cmd: Command = {
				id: "cmd-5",
				action: "click",
			};
			expect(validateCommand(cmd).valid).toBe(false);
		});

		it("should reject fill without value", () => {
			const cmd: Command = {
				id: "cmd-6",
				action: "fill",
				target: { selector: "#input" },
			};
			expect(validateCommand(cmd).valid).toBe(false);
		});
	});
});

describe("interaction auto-wait (DOM)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	function makeButton(id: string, disabled = false): HTMLButtonElement {
		const el = document.createElement("button");
		el.id = id;
		if (disabled) el.disabled = true;
		document.body.appendChild(el);
		return el;
	}

	it("click succeeds on an element that appears shortly after the call", async () => {
		const target = { selector: "#late" };
		setTimeout(() => {
			makeButton("late");
		}, 100);

		const result = await executeCommand({
			id: "c1",
			action: "click",
			target,
		});
		expect(result.success).toBe(true);
	});

	it("click on a never-appearing selector fails with 'not found' wording", async () => {
		const start = Date.now();
		const result = await executeCommand({
			id: "c2",
			action: "click",
			target: { selector: "#ghost" },
			options: { timeout: 200 },
		});
		const elapsed = Date.now() - start;
		expect(result.success).toBe(false);
		expect(result.error).toMatch(/not found/i);
		// Duration should be roughly the 200ms timeout, not the 5s default.
		expect(elapsed).toBeLessThan(1500);
	});

	it("fill on a disabled input fails with 'disabled' wording", async () => {
		makeButton("disabled-btn", true);
		const result = await executeCommand({
			id: "c3",
			action: "fill",
			target: { selector: "#disabled-btn" },
			value: "x",
			options: { timeout: 200 },
		});
		expect(result.success).toBe(false);
		expect(result.error).toMatch(/disabled/i);
	});

	it("honors a short options.timeout (200ms) instead of the 5s default", async () => {
		const start = Date.now();
		await executeCommand({
			id: "c4",
			action: "hover",
			target: { selector: "#missing" },
			options: { timeout: 200 },
		});
		const elapsed = Date.now() - start;
		expect(elapsed).toBeLessThan(1500);
	});

	it("getText on a missing element still fails instantly (probing semantics)", async () => {
		const start = Date.now();
		const result = await executeCommand({
			id: "c5",
			action: "getText",
			target: { selector: "#nope" },
		});
		const elapsed = Date.now() - start;
		expect(result.success).toBe(false);
		// Probing actions must not wait — should fail well under the 5s default.
		expect(elapsed).toBeLessThan(500);
	});
});

describe("wait action (DOM)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	it("resolves with element info when the element appears shortly after", async () => {
		setTimeout(() => {
			const el = document.createElement("div");
			el.id = "appears";
			el.textContent = "hello";
			document.body.appendChild(el);
		}, 100);

		const result = await executeCommand({
			id: "w1",
			action: "wait",
			target: { selector: "#appears" },
			options: { timeout: 2000 },
		});
		expect(result.success).toBe(true);
		expect((result.data as { text?: string }).text).toContain("hello");
	});

	it("fails with success:false and names the selector on timeout", async () => {
		const result = await executeCommand({
			id: "w2",
			action: "wait",
			target: { selector: "#never" },
			options: { timeout: 200 },
		});
		expect(result.success).toBe(false);
		expect(result.error).toMatch(/#never/);
	});

	it("waits even when waitForAppear is absent from the target", async () => {
		setTimeout(() => {
			const el = document.createElement("span");
			el.id = "late-span";
			document.body.appendChild(el);
		}, 100);

		const result = await executeCommand({
			id: "w3",
			action: "wait",
			// No `waitForAppear` — `wait` must still wait for appearance.
			target: { selector: "#late-span" },
			options: { timeout: 2000 },
		});
		expect(result.success).toBe(true);
	});
});

describe("scrollTo action (DOM)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	it("resolves (settles) without hanging when the target is present", async () => {
		const el = document.createElement("div");
		el.id = "scroll-target";
		document.body.appendChild(el);

		const start = Date.now();
		const result = await executeCommand({
			id: "s1",
			action: "scrollTo",
			target: { selector: "#scroll-target" },
			options: { timeout: 2000 },
		});
		expect(result.success).toBe(true);
		// The settle wait must terminate well within the timeout/hard cap.
		expect(Date.now() - start).toBeLessThan(2000);
	});
});

describe("evaluate action (DOM)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
		document.title = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	it("still evaluates single-expression scripts", async () => {
		document.title = "EvalPage";
		const result = await executeCommand({
			id: "e1",
			action: "evaluate",
			value: "document.title",
		});
		expect(result.success).toBe(true);
		expect(result.data).toBe("EvalPage");
	});

	it("evaluates multi-statement scripts with an explicit return", async () => {
		const result = await executeCommand({
			id: "e2",
			action: "evaluate",
			value: "const a = 2; const b = 3; return a + b;",
		});
		expect(result.success).toBe(true);
		expect(result.data).toBe(5);
	});

	it("awaits async scripts and returns the resolved value", async () => {
		const result = await executeCommand({
			id: "e3",
			action: "evaluate",
			value: "return await Promise.resolve(42);",
		});
		expect(result.success).toBe(true);
		expect(result.data).toBe(42);
	});

	it("propagates a script's own runtime error", async () => {
		const result = await executeCommand({
			id: "e4",
			action: "evaluate",
			value: "throw new Error('boom');",
		});
		expect(result.success).toBe(false);
		expect(result.error).toMatch(/boom/);
	});

	it("reports a SyntaxError for a script invalid in both modes", async () => {
		const result = await executeCommand({
			id: "e5",
			action: "evaluate",
			value: "const = = ;",
		});
		expect(result.success).toBe(false);
		expect(result.error).toMatch(/SyntaxError/);
	});
});
