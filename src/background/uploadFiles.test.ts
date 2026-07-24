import { describe, expect, it } from "bun:test";
import { resolveAndSetFiles } from "./uploadFiles";

describe("resolveAndSetFiles", () => {
	it("resolves the selector via DOM.* and sets files by nodeId", async () => {
		const calls: { method: string; params: Record<string, unknown> }[] = [];
		const send = async (method: string, params: Record<string, unknown>) => {
			calls.push({ method, params });
			switch (method) {
				case "DOM.getDocument":
					return { root: { nodeId: 1 } };
				case "DOM.querySelector":
					return { nodeId: 42 };
				default:
					return {};
			}
		};

		await resolveAndSetFiles(send, "#file", ["/tmp/a.png"]);

		const setCall = calls.find((c) => c.method === "DOM.setFileInputFiles");
		expect(setCall).toBeDefined();
		expect(setCall?.params.nodeId).toBe(42);
		expect(setCall?.params.files).toEqual(["/tmp/a.png"]);
	});

	it("throws when the selector matches nothing (nodeId 0)", async () => {
		const send = async (method: string) => {
			if (method === "DOM.getDocument") {
				return { root: { nodeId: 1 } };
			}
			return { nodeId: 0 };
		};

		await expect(
			resolveAndSetFiles(send, "#missing", ["/tmp/a.png"]),
		).rejects.toThrow("no element matched");
	});
});
