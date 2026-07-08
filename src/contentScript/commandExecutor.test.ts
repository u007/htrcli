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
