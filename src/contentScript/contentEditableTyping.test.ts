import { describe, expect, it } from "bun:test";
import "../test/domSetup";

/**
 * happy-dom does not implement:
 *   - document.execCommand
 *   - HTMLElement.isContentEditable (always false)
 *
 * The stubs below provide the two behaviours the content script relies on for
 * a contenteditable:
 *   - "insertText" inserts at the current selection
 *   - "selectAll" + "delete" clears the content
 *
 * They are installed per test so the stubbed isContentEditable only applies
 * to the element under test.
 */
function installContentEditableSupport(el: HTMLElement): void {
	Object.defineProperty(el, "isContentEditable", {
		configurable: true,
		get: () => true,
	});
	// biome-ignore lint/suspicious/noExplicitAny: test stub
	(document as any).execCommand = (
		command: string,
		_showUI?: boolean,
		value?: string,
	) => {
		const selection = document.getSelection();
		if (command === "insertText") {
			// Chromium treats an empty insertText as a no-op.
			if (!value) return false;
			if (!selection || selection.rangeCount === 0) return false;
			const range = selection.getRangeAt(0);
			range.deleteContents();
			const textNode = document.createTextNode(value ?? "");
			range.insertNode(textNode);
			range.setStartAfter(textNode);
			range.collapse(true);
			selection.removeAllRanges();
			selection.addRange(range);
			return true;
		}
		if (command === "selectAll") {
			if (!selection) return false;
			const range = document.createRange();
			range.selectNodeContents(
				(document.activeElement as HTMLElement) ??
					(document.body as HTMLElement),
			);
			selection.removeAllRanges();
			selection.addRange(range);
			return true;
		}
		if (command === "delete") {
			if (!selection || selection.rangeCount === 0) return false;
			selection.getRangeAt(0).deleteContents();
			return true;
		}
		return false;
	};
}

describe("contenteditable typing (append vs replace)", () => {
	it("appends to existing content instead of replacing it", async () => {
		document.body.innerHTML = `<div id="ce" contenteditable="true"><p>-- <br>Sent from my device</p></div>`;
		const el = document.getElementById("ce");
		if (!el) throw new Error("missing element");
		installContentEditableSupport(el);

		const { executeCommand } = await import("./commandExecutor");
		await executeCommand({
			id: "1",
			action: "type",
			target: { selector: "#ce" },
			value: "Reply body",
		});

		const text = (el.textContent ?? "").trim();
		expect(text).toContain("Sent from my device");
		expect(text).toContain("Reply body");
	});

	it("appends after the existing content, not before it", async () => {
		document.body.innerHTML = `<div id="ce" contenteditable="true">EXISTING</div>`;
		const el = document.getElementById("ce");
		if (!el) throw new Error("missing element");
		installContentEditableSupport(el);

		const { executeCommand } = await import("./commandExecutor");
		await executeCommand({
			id: "1",
			action: "type",
			target: { selector: "#ce" },
			value: "APPENDED",
		});

		expect((el.textContent ?? "").trim()).toBe("EXISTINGAPPENDED");
	});

	it("clears the content when a contenteditable is filled with an empty value", async () => {
		document.body.innerHTML = `<div id="ce" contenteditable="true">OLD CONTENT</div>`;
		const el = document.getElementById("ce");
		if (!el) throw new Error("missing element");
		installContentEditableSupport(el);

		const { executeCommand } = await import("./commandExecutor");
		const res = await executeCommand({
			id: "1",
			action: "fill",
			target: { selector: "#ce" },
			value: "",
		});

		expect(res.success).toBe(true);
		expect((el.textContent ?? "").trim()).toBe("");
	});

	it("still replaces the content for fill", async () => {
		document.body.innerHTML = `<div id="ce" contenteditable="true">EXISTING</div>`;
		const el = document.getElementById("ce");
		if (!el) throw new Error("missing element");
		installContentEditableSupport(el);

		const { executeCommand } = await import("./commandExecutor");
		await executeCommand({
			id: "1",
			action: "fill",
			target: { selector: "#ce" },
			value: "REPLACED",
		});

		expect((el.textContent ?? "").trim()).toBe("REPLACED");
	});
});

/**
 * A browser's execCommand("selectAll") selects the whole EDITING HOST, not the
 * target element. This stub models that exactly, so a fix that still relies on
 * selectAll cannot pass.
 */
function installContentEditableSupportSelectingHost(host: HTMLElement): void {
	for (const el of [host, ...Array.from(host.querySelectorAll("*"))]) {
		Object.defineProperty(el, "isContentEditable", {
			configurable: true,
			get: () => true,
		});
	}
	// biome-ignore lint/suspicious/noExplicitAny: test stub
	(document as any).execCommand = (
		command: string,
		_showUI?: boolean,
		value?: string,
	) => {
		const selection = document.getSelection();
		if (command === "selectAll") {
			if (!selection) return false;
			const range = document.createRange();
			range.selectNodeContents(host);
			selection.removeAllRanges();
			selection.addRange(range);
			return true;
		}
		if (command === "delete") {
			if (!selection || selection.rangeCount === 0) return false;
			selection.getRangeAt(0).deleteContents();
			return true;
		}
		if (command === "insertText") {
			// Chromium treats an empty insertText as a no-op.
			if (!value) return false;
			if (!selection || selection.rangeCount === 0) return false;
			const range = selection.getRangeAt(0);
			range.deleteContents();
			const textNode = document.createTextNode(value ?? "");
			range.insertNode(textNode);
			range.setStartAfter(textNode);
			range.collapse(true);
			selection.removeAllRanges();
			selection.addRange(range);
			return true;
		}
		return false;
	};
}

describe("contenteditable fill is scoped to the target", () => {
	it("replaces only the target's own content and keeps its siblings", async () => {
		document.body.innerHTML = `
			<div id="host" contenteditable="true">
				<p id="q">QUOTED THREAD THAT MUST SURVIVE</p>
				<p class="target">TARGET</p>
				<p id="s">SIGNATURE</p>
			</div>`;
		const host = document.getElementById("host");
		if (!host) throw new Error("missing host");
		installContentEditableSupportSelectingHost(host);

		const { executeCommand } = await import("./commandExecutor");
		await executeCommand({
			id: "1",
			action: "fill",
			target: { selector: ".target" },
			value: "NEW",
		});

		const text = host.textContent ?? "";
		expect(text).toContain("QUOTED THREAD THAT MUST SURVIVE");
		expect(text).toContain("SIGNATURE");
		expect(text).toContain("NEW");
	});
});

describe("prepareKeys places the caret at the end of a contenteditable", () => {
	it("collapses the selection after the existing content", async () => {
		document.body.innerHTML = `<div id="ce" contenteditable="true"><p>QUOTED</p><p>SIGNATURE</p></div>`;
		const el = document.getElementById("ce");
		if (!el) throw new Error("missing element");
		installContentEditableSupportSelectingHost(el);

		const { executeCommand } = await import("./commandExecutor");
		await executeCommand({
			id: "1",
			action: "prepareKeys",
			target: { selector: "#ce" },
		});

		const selection = document.getSelection();
		expect(selection).not.toBeNull();
		if (!selection || selection.rangeCount === 0) throw new Error("no range");
		const range = selection.getRangeAt(0);
		expect(range.collapsed).toBe(true);
		// Collapsed at the END of the element's content.
		expect(range.startContainer).toBe(el);
		expect(range.startOffset).toBe(el.childNodes.length);
	});
});
