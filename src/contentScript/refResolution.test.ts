import { beforeEach, describe, expect, it } from "bun:test";
import "../test/domSetup";
import { executeCommand } from "./commandExecutor";
import { clearRefs, refCount } from "./refRegistry";

describe("ref resolution end-to-end", () => {
	beforeEach(() => {
		clearRefs();
		document.body.innerHTML = "";
	});

	it("find --ref mints a ref, and a later command resolves it", async () => {
		const btn = document.createElement("button");
		btn.id = "go";
		btn.textContent = "Go";
		document.body.appendChild(btn);

		const findRes = await executeCommand({
			id: "1",
			action: "find",
			target: { selector: "#go" },
			options: { assignRef: true },
		});
		expect(findRes.success).toBe(true);
		const info = findRes.data as { ref?: string };
		expect(info.ref).toBe("@e1");

		// A later getText addressed purely by the ref resolves to #go.
		const textRes = await executeCommand({
			id: "2",
			action: "getText",
			target: { ref: "@e1" },
		});
		expect(textRes.success).toBe(true);
		expect(textRes.data).toBe("Go");
	});

	it("findAll --ref mints refs for every match", async () => {
		for (let i = 1; i <= 3; i++) {
			const btn = document.createElement("button");
			btn.textContent = `Item ${i}`;
			document.body.appendChild(btn);
		}

		await executeCommand({
			id: "1",
			action: "findAll",
			target: { selector: "button" },
			options: { assignRef: true },
		});
		expect(refCount()).toBe(3);
	});

	it("stale ref produces an error, not a wrong element", async () => {
		const btn = document.createElement("button");
		btn.textContent = "Do not click";
		document.body.appendChild(btn);

		// Mint a ref
		await executeCommand({
			id: "1",
			action: "find",
			target: { selector: "button" },
			options: { assignRef: true },
		});

		// Detach the element
		btn.remove();

		const clickRes = await executeCommand({
			id: "2",
			action: "click",
			target: { ref: "@e1" },
		});
		expect(clickRes.success).toBe(false);
		expect(clickRes.error).toMatch(/stale ref/);
	});

	it("unknown ref on a different page errors", async () => {
		const res = await executeCommand({
			id: "1",
			action: "click",
			target: { ref: "@e999" },
		});
		expect(res.success).toBe(false);
		expect(res.error).toMatch(/stale ref/);
	});
});
