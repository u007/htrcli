import { describe, expect, it } from "bun:test";
import { resolveDialog } from "./dialogPolicy";

describe("resolveDialog", () => {
	it("accepts when policy is accept", () => {
		const r = resolveDialog({ action: "accept" }, "confirm", "Sure?");
		expect(r.accept).toBe(true);
		expect(r.promptText).toBeUndefined();
		expect(r.entry).toEqual({
			dialogType: "confirm",
			message: "Sure?",
			resolvedAction: "accept",
		});
	});

	it("dismisses when policy is dismiss", () => {
		const r = resolveDialog({ action: "dismiss" }, "confirm", "Sure?");
		expect(r.accept).toBe(false);
		expect(r.entry.resolvedAction).toBe("dismiss");
	});

	it("responds with text for a prompt", () => {
		const r = resolveDialog(
			{ action: "respond", text: "hello" },
			"prompt",
			"Name?",
		);
		expect(r.accept).toBe(true);
		expect(r.promptText).toBe("hello");
		expect(r.entry).toEqual({
			dialogType: "prompt",
			message: "Name?",
			resolvedAction: "accept",
			respondedText: "hello",
		});
	});

	it("treats respond with no text as an empty string response", () => {
		const r = resolveDialog({ action: "respond" }, "prompt", "Name?");
		expect(r.accept).toBe(true);
		expect(r.promptText).toBe("");
		expect(r.entry.respondedText).toBe("");
	});
});
