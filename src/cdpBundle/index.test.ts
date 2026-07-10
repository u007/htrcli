import { beforeEach, describe, expect, test } from "bun:test";
import "../test/domSetup";
import "./index";

describe("__htcliDom bundle global", () => {
	beforeEach(() => {
		document.body.innerHTML = `<input id="email" type="text" />`;
	});

	test("exposes exec on window", () => {
		expect(typeof window.__htcliDom?.exec).toBe("function");
	});

	test("fill via exec sets value and fires input event", async () => {
		let inputFired = false;
		const el = document.querySelector<HTMLInputElement>("#email");
		if (!el) {
			throw new Error("missing #email input");
		}
		el.addEventListener("input", () => {
			inputFired = true;
		});
		const dom = window.__htcliDom;
		if (!dom) {
			throw new Error("__htcliDom not installed");
		}
		const result = await dom.exec({
			id: "1",
			action: "fill",
			target: { selector: "#email" },
			value: "james@mercstudio.com",
		});
		expect(result.success).toBe(true);
		expect(el.value).toBe("james@mercstudio.com");
		expect(inputFired).toBe(true);
	});
});
