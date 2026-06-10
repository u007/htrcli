import { describe, expect, it } from "bun:test";
import type { Command, CommandAction } from "../types/commands";

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
