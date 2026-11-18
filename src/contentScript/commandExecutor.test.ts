import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import "../test/domSetup";
import type { Command, CommandAction, SnapshotNode } from "../types/commands";
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
	"clear",
	"select",
	"check",
	"uncheck",
	"pressKey",
	"selectText",
	"highlight",
	"isChecked",
	"pick",
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
			"isChecked",
			"pick",
		];

		// `type` is deliberately absent: without a target it types into the focused
		// editable element, so it is covered by its own tests below.
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
			"snapshot",
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

describe("wait --gone (DOM)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	it("resolves once a matching element is removed", async () => {
		const el = document.createElement("div");
		el.id = "archivable";
		document.body.appendChild(el);
		setTimeout(() => el.remove(), 120);

		const result = await executeCommand({
			id: "g1",
			action: "wait",
			target: { selector: "#archivable" },
			options: { timeout: 2000, gone: true, interval: 50 },
		});
		expect(result.success).toBe(true);
		expect(result.data).toEqual({ gone: true });
	});

	it("resolves immediately when nothing matches", async () => {
		const result = await executeCommand({
			id: "g2",
			action: "wait",
			target: { selector: "#absent-from-start" },
			options: { timeout: 2000, gone: true, interval: 50 },
		});
		expect(result.success).toBe(true);
	});

	it("fails loudly, naming the selector, when the element stays", async () => {
		const el = document.createElement("div");
		el.id = "sticky";
		document.body.appendChild(el);

		const result = await executeCommand({
			id: "g3",
			action: "wait",
			target: { selector: "#sticky" },
			options: { timeout: 250, gone: true, interval: 50 },
		});
		expect(result.success).toBe(false);
		expect(result.error).toMatch(/#sticky/);
		expect(result.error).toMatch(/still present/);
	});

	it("does not report gone while the document is still loading", async () => {
		Object.defineProperty(document, "readyState", {
			configurable: true,
			get: () => "loading",
		});
		try {
			const result = await executeCommand({
				id: "g8",
				action: "wait",
				target: { selector: "#not-rendered-yet" },
				options: { timeout: 250, gone: true, interval: 50 },
			});
			expect(result.success).toBe(false);
			expect(result.error).toMatch(/still loading/);
		} finally {
			delete (document as { readyState?: string }).readyState;
		}
	});

	it("ignores target.index: gone means no match at all", async () => {
		const first = document.createElement("div");
		first.className = "row";
		const second = document.createElement("div");
		second.className = "row";
		document.body.appendChild(first);
		document.body.appendChild(second);
		setTimeout(() => second.remove(), 120);

		// Index 1 is the second row; removing it must not pass while the first
		// row still matches the selector.
		const result = await executeCommand({
			id: "g5",
			action: "wait",
			target: { selector: ".row", index: 1 },
			options: { timeout: 250, gone: true, interval: 50 },
		});
		expect(result.success).toBe(false);
		expect(result.error).toMatch(/still present/);
	});

	it("rejects gone without a positive interval", async () => {
		const result = await executeCommand({
			id: "g6",
			action: "wait",
			target: { selector: "#anything" },
			options: { timeout: 250, gone: true },
		});
		expect(result.success).toBe(false);
		expect(result.error).toMatch(/--interval must be > 0/);
	});

	it("still treats a hidden element as present", async () => {
		const el = document.createElement("div");
		el.id = "hidden-but-present";
		el.style.display = "none";
		document.body.appendChild(el);

		const result = await executeCommand({
			id: "g4",
			action: "wait",
			target: { selector: "#hidden-but-present" },
			options: { timeout: 250, gone: true, interval: 50 },
		});
		expect(result.success).toBe(false);
	});
});

describe("low-level synthetic input", () => {
	let originalElementFromPoint: typeof document.elementFromPoint;

	beforeEach(() => {
		document.body.innerHTML = "";
		originalElementFromPoint = document.elementFromPoint;
	});

	afterEach(() => {
		document.body.innerHTML = "";
		document.elementFromPoint = originalElementFromPoint;
	});

	it("dispatches keyDown and keyUp with the resolved key payload", async () => {
		const input = document.createElement("input");
		input.id = "key-target";
		document.body.appendChild(input);
		const events: KeyboardEvent[] = [];
		input.addEventListener("keydown", (event) => events.push(event));
		input.addEventListener("keyup", (event) => events.push(event));

		const down = await executeCommand({
			id: "key-down",
			action: "keyDown",
			target: { selector: "#key-target" },
			value: "Enter",
		});
		const up = await executeCommand({
			id: "key-up",
			action: "keyUp",
			target: { selector: "#key-target" },
			value: "Enter",
		});

		expect(down.success).toBe(true);
		expect(up.success).toBe(true);
		expect(events.map((event) => event.type)).toEqual(["keydown", "keyup"]);
		expect(events[0].key).toBe("Enter");
		expect(events[0].code).toBe("Enter");
		expect(events[0].keyCode).toBe(13);
		expect(events[1].keyCode).toBe(13);
	});

	it("routes coordinate mouseDown, mouseUp, and mouseMove through hit-testing", async () => {
		const target = document.createElement("button");
		target.id = "coordinate-target";
		document.body.appendChild(target);
		document.elementFromPoint = ((x: number, y: number) => {
			expect(x).toBe(25);
			expect(y).toBe(35);
			return target;
		}) as typeof document.elementFromPoint;
		const events: MouseEvent[] = [];
		target.addEventListener("mousedown", (event) => events.push(event));
		target.addEventListener("mouseup", (event) => events.push(event));
		target.addEventListener("mousemove", (event) => events.push(event));

		for (const action of ["mouseDown", "mouseUp", "mouseMove"] as const) {
			const result = await executeCommand({
				id: action,
				action,
				target: { x: 25, y: 35 },
			});
			expect(result.success).toBe(true);
		}

		expect(events.map((event) => event.type)).toEqual([
			"mousedown",
			"mouseup",
			"mousemove",
		]);
		// The listeners are installed on `target`; receiving all three events here
		// proves coordinate hit-testing routed them to that element rather than body.
		expect(
			events.every((event) => event.clientX === 25 && event.clientY === 35),
		).toBe(true);
	});

	it("routes drag source, intermediate, and destination events to hit-tested elements", async () => {
		const source = document.createElement("div");
		source.id = "drag-source";
		const destination = document.createElement("div");
		destination.id = "drag-destination";
		document.body.append(source, destination);
		document.elementFromPoint = ((x: number) =>
			x < 50 ? source : destination) as typeof document.elementFromPoint;
		const events: string[] = [];
		for (const element of [source, destination]) {
			for (const type of ["mousedown", "mousemove", "mouseup"] as const) {
				element.addEventListener(type, (event) =>
					events.push(`${event.type}:${element.id}:${event.clientX}`),
				);
			}
		}

		const result = await executeCommand({
			id: "drag",
			action: "drag",
			target: { x: 10, y: 10 },
			options: { endTarget: { x: 90, y: 90 }, steps: 2 },
		});

		expect(result.success).toBe(true);
		expect(events).toEqual([
			"mousedown:drag-source:10",
			"mousemove:drag-destination:50",
			"mousemove:drag-destination:90",
			"mouseup:drag-destination:90",
		]);
	});

	it("reads both drag endpoints only after both are scrolled (press lands on the source's final position)", async () => {
		// A simulated page: each element has a fixed document top, and every
		// scroll moves all of them. Scrolling to the destination moves the source,
		// so a source point read before that scroll is stale.
		const viewportHeight = window.innerHeight;
		let scrollY = 0;
		const originalScrollBy = window.scrollBy;
		window.scrollBy = ((opts: { top: number }) => {
			scrollY += opts.top;
		}) as typeof window.scrollBy;
		const place = (id: string, docTop: number): HTMLElement => {
			const el = document.createElement("div");
			el.id = id;
			document.body.append(el);
			el.getBoundingClientRect = () => {
				const top = docTop - scrollY;
				return {
					x: 0,
					y: top,
					top,
					bottom: top + 40,
					left: 0,
					right: 40,
					width: 40,
					height: 40,
					toJSON() {},
				} as DOMRect;
			};
			el.scrollIntoView = () => {
				scrollY = docTop + 20 - viewportHeight / 2;
			};
			return el;
		};
		const source = place("drag-source-far", 300);
		const destination = place("drag-dest-far", 700);
		const events: string[] = [];
		for (const element of [source, destination]) {
			for (const type of ["mousedown", "mousemove", "mouseup"] as const) {
				element.addEventListener(type, (event) =>
					events.push(`${event.type}:${element.id}:${event.clientY}`),
				);
			}
		}

		try {
			const result = await executeCommand({
				id: "drag-both-scrolled",
				action: "drag",
				target: { selector: "#drag-source-far" },
				options: { endTarget: { selector: "#drag-dest-far" }, steps: 1 },
			});
			expect(result.success).toBe(true);
		} finally {
			window.scrollBy = originalScrollBy;
		}

		// Both centred, then one fit-scroll for the pair: the endpoints end up
		// 200px either side of the viewport centre.
		expect(events).toEqual([
			`mousedown:drag-source-far:${viewportHeight / 2 - 200}`,
			`mousemove:drag-dest-far:${viewportHeight / 2 + 200}`,
			`mouseup:drag-dest-far:${viewportHeight / 2 + 200}`,
		]);
	});

	it("returns explicit errors for missing coordinate hit targets and destinations", async () => {
		document.elementFromPoint = (() =>
			null) as typeof document.elementFromPoint;
		const noHit = await executeCommand({
			id: "no-hit",
			action: "mouseDown",
			target: { x: 1, y: 2 },
		});
		expect(noHit.success).toBe(false);
		expect(noHit.error).toMatch(/no element exists/i);

		const missingDestination = await executeCommand({
			id: "missing-destination",
			action: "drag",
			target: { selector: "#source" },
		});
		expect(missingDestination.success).toBe(false);
		expect(missingDestination.error).toMatch(/destination selector/i);
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

describe("snapshot action (DOM)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
		document.title = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
		document.title = "";
	});

	it("builds a tree with accessible names, states, and refs", async () => {
		document.title = "Snapshot page";
		document.body.innerHTML = `
			<main>
				<h1>Dashboard</h1>
				<button id="save" aria-pressed="true">Save</button>
				<label for="name">Name</label>
				<input id="name" type="text" value="Ada" />
				<section aria-label="Overview">
					<p>Summary text</p>
				</section>
			</main>
		`;

		const result = await executeCommand({
			id: "snap1",
			action: "snapshot",
		});

		expect(result.success).toBe(true);
		const snapshot = result.data as SnapshotNode;
		expect(snapshot.role).toBe("document");
		expect(snapshot.name).toBe("Snapshot page");

		const main = snapshot.children?.find((node) => node.role === "main");
		expect(main).toBeTruthy();
		const mainNode = main as SnapshotNode;

		const button = mainNode.children?.find((node) => node.role === "button");
		expect(button).toBeTruthy();
		const buttonNode = button as SnapshotNode;
		expect(buttonNode.name).toBe("Save");
		expect(buttonNode.ref).toMatch(/^@e\d+$/);
		expect(buttonNode.state?.pressed).toBe(true);

		const input = mainNode.children?.find((node) => node.role === "textbox");
		expect(input).toBeTruthy();
		const inputNode = input as SnapshotNode;
		expect(inputNode.name).toBe("Name");
		expect(inputNode.value).toBe("Ada");
		expect(inputNode.ref).toMatch(/^@e\d+$/);

		const region = mainNode.children?.find((node) => node.role === "region");
		expect(region).toBeTruthy();
		const regionNode = region as SnapshotNode;
		expect(regionNode.name).toBe("Overview");
		expect(regionNode.ref).toMatch(/^@e\d+$/);
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

describe("prepareClick / prepareKeys (internal CDP preparation)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	function makeButton(id: string, hidden = false): HTMLButtonElement {
		const el = document.createElement("button");
		el.id = id;
		if (hidden) el.style.display = "none";
		document.body.appendChild(el);
		return el;
	}

	function makeInput(id: string): HTMLInputElement {
		const el = document.createElement("input");
		el.id = id;
		document.body.appendChild(el);
		return el;
	}

	it("prepareClick returns center coordinates for a visible button", async () => {
		makeButton("prep-btn");
		const result = await executeCommand({
			id: "pc1",
			action: "prepareClick",
			target: { selector: "#prep-btn" },
		});
		expect(result.success).toBe(true);
		const data = result.data as { x: number; y: number };
		// domSetup stubs a 100x50 rect at (0,0), so the center is (50, 25).
		expect(data).toEqual({ x: 50, y: 25 });
	});

	it("prepareClick fails with an actionability error for a hidden element", async () => {
		makeButton("prep-hidden", true);
		const result = await executeCommand({
			id: "pc2",
			action: "prepareClick",
			target: { selector: "#prep-hidden" },
			options: { timeout: 200 },
		});
		expect(result.success).toBe(false);
		expect(result.error).toMatch(/not visible/i);
	});

	it("prepareKeys leaves the target focused", async () => {
		const input = makeInput("prep-input");
		const result = await executeCommand({
			id: "pk1",
			action: "prepareKeys",
			target: { selector: "#prep-input" },
		});
		expect(result.success).toBe(true);
		const data = result.data as { focused: boolean };
		expect(data.focused).toBe(true);
		expect(document.activeElement).toBe(input);
	});

	it("prepareKeys without a target succeeds using the focused element", async () => {
		const input = makeInput("prep-focused");
		input.focus();
		const result = await executeCommand({
			id: "pk2",
			action: "prepareKeys",
		});
		expect(result.success).toBe(true);
		const data = result.data as { focused: boolean };
		expect(data.focused).toBe(true);
		// Targetless prep must not steal focus from the current element.
		expect(document.activeElement).toBe(input);
	});

	it("pressKey without a target dispatches to the focused element", async () => {
		const input = makeInput("press-focused");
		input.focus();
		const keys: string[] = [];
		input.addEventListener("keydown", (e) => keys.push(e.code));
		const result = await executeCommand({
			id: "pk3",
			action: "pressKey",
			value: "Enter",
		});
		expect(result.success).toBe(true);
		expect(keys).toEqual(["Enter"]);
	});
});

describe("synthetic input upgrade (Firefox / fallback path)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	function makeButton(id: string) {
		const el = document.createElement("button");
		el.id = id;
		document.body.appendChild(el);
		return el;
	}

	function makeInput(id: string) {
		const el = document.createElement("input");
		el.id = id;
		document.body.appendChild(el);
		return el;
	}

	it("click dispatches pointerdown before mousedown and pointerup before mouseup", async () => {
		const btn = makeButton("ptr-btn");
		const seen: string[] = [];
		const types = [
			"pointerover",
			"mouseover",
			"mouseenter",
			"pointerdown",
			"mousedown",
			"pointerup",
			"mouseup",
			"click",
		];
		for (const t of types) {
			btn.addEventListener(t, () => seen.push(t));
		}

		const result = await executeCommand({
			id: "s1",
			action: "click",
			target: { selector: "#ptr-btn" },
		});
		expect(result.success).toBe(true);
		expect(seen).toContain("pointerdown");
		expect(seen).toContain("mousedown");
		expect(seen).toContain("pointerup");
		expect(seen).toContain("mouseup");
		expect(seen.indexOf("pointerdown")).toBeLessThan(seen.indexOf("mousedown"));
		expect(seen.indexOf("pointerup")).toBeLessThan(seen.indexOf("mouseup"));
	});

	it("synthetic Enter keydown carries code 'Enter' (not 'KeyEnter')", async () => {
		const input = makeInput("enter-input");
		let keydownCode = "";
		input.addEventListener("keydown", (e) => {
			keydownCode = e.code;
		});

		const result = await executeCommand({
			id: "s2",
			action: "pressKey",
			target: { selector: "#enter-input" },
			value: "Enter",
		});
		expect(result.success).toBe(true);
		expect(keydownCode).toBe("Enter");
	});

	it("synthetic type emits KeyX code for a letter char", async () => {
		const input = makeInput("type-input");
		let keydownCode = "";
		input.addEventListener("keydown", (e) => {
			keydownCode = e.code;
		});

		const result = await executeCommand({
			id: "s3",
			action: "type",
			target: { selector: "#type-input" },
			value: "a",
		});
		expect(result.success).toBe(true);
		expect(keydownCode).toBe("KeyA");
	});
});

describe("uploadFiles action (Firefox content-script path)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	it("sets files on an <input type=file> via DataTransfer and dispatches change", async () => {
		const input = document.createElement("input");
		input.type = "file";
		input.id = "file-input";
		document.body.appendChild(input);

		let changed = 0;
		input.addEventListener("change", () => {
			changed++;
		});

		const result = await executeCommand({
			id: "up1",
			action: "uploadFiles",
			target: { selector: "#file-input" },
			options: {
				filesData: [
					{
						name: "addon.xpi",
						mimeType: "application/x-xpinstall",
						base64: btoa("hello world"),
					},
					{
						name: "src.zip",
						mimeType: "application/zip",
						base64: btoa("zip-bytes"),
					},
				],
			},
		});

		expect(result.success).toBe(true);
		expect(changed).toBe(1);
		const files = (result.data as { files: string[] }).files;
		expect(files).toEqual(["addon.xpi", "src.zip"]);
		expect(input.files?.length).toBe(2);
		expect(input.files?.[0]?.name).toBe("addon.xpi");
		expect(input.files?.[1]?.name).toBe("src.zip");
	});

	it("fails when the target is not a file input", async () => {
		const text = document.createElement("input");
		text.type = "text";
		text.id = "text-input";
		document.body.appendChild(text);

		const result = await executeCommand({
			id: "up2",
			action: "uploadFiles",
			target: { selector: "#text-input" },
			options: {
				filesData: [{ name: "a.xpi", base64: btoa("x") }],
			},
		});
		expect(result.success).toBe(false);
		expect(result.error).toContain("not an <input type=file>");
	});

	it("fails when filesData is missing (Chrome handles this action in the background)", async () => {
		const input = document.createElement("input");
		input.type = "file";
		input.id = "file-input-2";
		document.body.appendChild(input);

		const result = await executeCommand({
			id: "up3",
			action: "uploadFiles",
			target: { selector: "#file-input-2" },
		});
		expect(result.success).toBe(false);
		expect(result.error).toContain("filesData");
	});
});

describe("type without a target (DOM)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	it("types into the focused editable element", async () => {
		const input = document.createElement("input");
		document.body.appendChild(input);
		input.focus();

		const result = await executeCommand({
			id: "type-focus-1",
			action: "type",
			value: "abc",
		});

		expect(result.success).toBe(true);
		expect(input.value).toBe("abc");
	});

	it("refuses when the focused element is not editable", async () => {
		const button = document.createElement("button");
		document.body.appendChild(button);
		button.focus();

		const result = await executeCommand({
			id: "type-focus-2",
			action: "type",
			value: "abc",
		});

		expect(result.success).toBe(false);
		expect(result.error).toBe(
			"type without a target needs a focused editable element",
		);
	});
});

describe("scrollBy action (DOM)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	it("scrolls an element by dx/dy and reports the position after", async () => {
		const box = document.createElement("div");
		box.id = "scroll-box";
		document.body.appendChild(box);
		// happy-dom has no Element.scrollBy; model the browser's effect on scrollLeft/Top.
		box.scrollBy = ((opts: ScrollToOptions) => {
			box.scrollLeft += opts.left ?? 0;
			box.scrollTop += opts.top ?? 0;
		}) as typeof box.scrollBy;

		const result = await executeCommand({
			id: "sb-1",
			action: "scrollBy",
			target: { selector: "#scroll-box" },
			options: { dx: 0, dy: 40 },
		});

		expect(result.success).toBe(true);
		expect(result.data).toEqual({ x: 0, y: 40 });
	});

	it("rejects a non-numeric dy", async () => {
		const result = await executeCommand({
			id: "sb-2",
			action: "scrollBy",
			options: { dx: 0, dy: "40" },
		});

		expect(result.success).toBe(false);
		expect(result.error).toBe(
			'Action "scrollBy" requires options.dy as a finite number',
		);
	});
});

describe("isChecked action (DOM)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	it("reads a checkbox and a radio from their checked property", async () => {
		document.body.innerHTML =
			'<input id="cb" type="checkbox" checked><input id="rd" type="radio">';

		const box = await executeCommand({
			id: "ic-1",
			action: "isChecked",
			target: { selector: "#cb" },
		});
		const radio = await executeCommand({
			id: "ic-2",
			action: "isChecked",
			target: { selector: "#rd" },
		});

		expect(box.data).toEqual({ checked: true });
		expect(radio.data).toEqual({ checked: false });
	});

	it("reads aria-checked: true is checked, false and mixed are not", async () => {
		document.body.innerHTML =
			'<div id="t" role="checkbox" aria-checked="true"></div>' +
			'<div id="f" role="checkbox" aria-checked="false"></div>' +
			'<div id="m" role="checkbox" aria-checked="mixed"></div>';

		const results = await Promise.all(
			["#t", "#f", "#m"].map((selector) =>
				executeCommand({
					id: `ic-${selector}`,
					action: "isChecked",
					target: { selector },
				}),
			),
		);

		expect(results.map((r) => r.data)).toEqual([
			{ checked: true },
			{ checked: false },
			{ checked: false },
		]);
	});

	it("throws for an element that is not checkable", async () => {
		document.body.innerHTML = '<div id="plain" role="checkbox"></div>';

		const result = await executeCommand({
			id: "ic-3",
			action: "isChecked",
			target: { selector: "#plain" },
		});

		expect(result.success).toBe(false);
		expect(result.error).toBe("element is not checkable");
	});
});

describe("pick action (DOM)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	const SELECT_HTML =
		'<select id="colour">' +
		'<option value="r">Red</option>' +
		'<option value="a">Alpha</option>' +
		'<option value="b">Alphabet</option>' +
		'<option value="g">Green</option>' +
		"</select>";

	it("picks the single best-ranked native <select> option", async () => {
		document.body.innerHTML = SELECT_HTML;

		const result = await executeCommand({
			id: "pick-1",
			action: "pick",
			target: { selector: "#colour" },
			value: "Green",
		});

		expect(result.success).toBe(true);
		expect(result.data).toEqual({ value: "g", matched: "Green" });
		expect((document.querySelector("#colour") as HTMLSelectElement).value).toBe(
			"g",
		);
	});

	it("errors on an ambiguous native <select> match and lists the tie", async () => {
		document.body.innerHTML = SELECT_HTML;

		const result = await executeCommand({
			id: "pick-2",
			action: "pick",
			target: { selector: "#colour" },
			value: "Alph",
		});

		expect(result.success).toBe(false);
		expect(result.error).toContain("ambiguous");
		expect(result.error).toContain('"Alpha"');
		expect(result.error).toContain('"Alphabet"');
	});

	it("errors on a native <select> with no match and lists the options seen", async () => {
		document.body.innerHTML = SELECT_HTML;

		const result = await executeCommand({
			id: "pick-3",
			action: "pick",
			target: { selector: "#colour" },
			value: "Purple",
		});

		expect(result.success).toBe(false);
		expect(result.error).toContain('no option matches "Purple"');
		expect(result.error).toContain('"Red"');
	});

	it("picks a combobox option and reads the displayed value back", async () => {
		document.body.innerHTML =
			'<div class="select__control"><div class="select__value-container">' +
			'<div class="select__single-value">Choose</div>' +
			'<input id="city" role="combobox" aria-controls="city-list" />' +
			"</div></div>" +
			'<div id="city-list" role="listbox">' +
			'<div role="option" id="o-paris">Paris</div>' +
			'<div role="option" id="o-parma">Parma</div>' +
			"</div>";
		const input = document.querySelector("#city") as HTMLInputElement;
		const single = document.querySelector(
			".select__single-value",
		) as HTMLElement;
		const listbox = document.querySelector("#city-list") as HTMLElement;
		// Typing filters the list; picking an option shows it as the selected value.
		input.addEventListener("input", () => {
			for (const option of Array.from(listbox.children) as HTMLElement[]) {
				option.style.display = option.textContent?.includes(input.value)
					? ""
					: "none";
			}
		});
		listbox.addEventListener("click", (event) => {
			const option = (event.target as HTMLElement).closest('[role="option"]');
			if (option) single.textContent = option.textContent;
		});

		const result = await executeCommand({
			id: "pick-4",
			action: "pick",
			target: { selector: "#city" },
			value: "Paris",
		});

		expect(result.success).toBe(true);
		expect(result.data).toEqual({ value: "Paris", matched: "Paris" });
	});
});

describe("click at viewport coordinates (DOM)", () => {
	let originalElementFromPoint: typeof document.elementFromPoint;

	beforeEach(() => {
		document.body.innerHTML = "";
		originalElementFromPoint = document.elementFromPoint;
	});

	afterEach(() => {
		document.elementFromPoint = originalElementFromPoint;
		document.body.innerHTML = "";
	});

	it("dispatches the click on the element at the point", async () => {
		const button = document.createElement("button");
		document.body.appendChild(button);
		let clicks = 0;
		button.addEventListener("click", () => {
			clicks++;
		});
		document.elementFromPoint = (() =>
			button) as typeof document.elementFromPoint;

		const result = await executeCommand({
			id: "xy-1",
			action: "click",
			target: { x: 12, y: 34 },
		});

		expect(result.success).toBe(true);
		expect(clicks).toBe(1);
	});

	it("errors when nothing is at the point", async () => {
		document.elementFromPoint = (() =>
			null) as typeof document.elementFromPoint;

		const result = await executeCommand({
			id: "xy-2",
			action: "click",
			target: { x: 12, y: 34 },
		});

		expect(result.success).toBe(false);
		expect(result.error).toBe("nothing at (12, 34)");
	});
});

describe("options.frame (DOM)", () => {
	/**
	 * An iframe whose document is a stand-in. happy-dom loads iframe pages on its
	 * own schedule, and under the full suite `contentDocument` stays null, so these
	 * tests pin the lookup logic rather than happy-dom's iframe loading. The
	 * stand-in is same-realm: cross-realm behaviour needs a live browser check.
	 */
	function makeFrame(id: string): {
		frame: HTMLIFrameElement;
		inner: Document;
	} {
		const frame = document.createElement("iframe");
		frame.id = id;
		document.body.appendChild(frame);
		const inner = document.implementation.createHTMLDocument("frame");
		Object.defineProperty(inner, "defaultView", { value: window });
		Object.defineProperty(frame, "contentDocument", {
			configurable: true,
			get: () => inner,
		});
		return { frame, inner };
	}

	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	it("resolves a target inside the frame's document", async () => {
		const { inner } = makeFrame("frame-a");
		inner.body.innerHTML = '<button id="inner-btn">Go</button>';

		const result = await executeCommand({
			id: "fr-1",
			action: "getText",
			target: { selector: "#inner-btn" },
			options: { frame: "#frame-a" },
		});

		expect(result.success).toBe(true);
		expect(result.data).toBe("Go");
	});

	it("does not see frame content without options.frame", async () => {
		const { inner } = makeFrame("frame-b");
		inner.body.innerHTML = '<button id="only-inside">Go</button>';

		const result = await executeCommand({
			id: "fr-2",
			action: "getText",
			target: { selector: "#only-inside" },
		});

		expect(result.success).toBe(false);
	});

	it("fills an input inside the frame's document", async () => {
		const { inner } = makeFrame("frame-c");
		inner.body.innerHTML = '<input id="inner-input">';

		const result = await executeCommand({
			id: "fr-3",
			action: "fill",
			target: { selector: "#inner-input" },
			value: "hello",
			options: { frame: "#frame-c" },
		});

		expect(result.success).toBe(true);
		expect(
			(inner.querySelector("#inner-input") as HTMLInputElement).value,
		).toBe("hello");
	});

	it("mints a ref inside the frame that later commands resolve without the frame", async () => {
		const { inner } = makeFrame("frame-r");
		inner.body.innerHTML = '<button id="ref-btn">Save</button>';

		const found = await executeCommand({
			id: "fr-ref-1",
			action: "find",
			target: { selector: "#ref-btn" },
			options: { frame: "#frame-r", assignRef: true },
		});
		const ref = (found.data as { ref?: string }).ref;
		expect(ref).toMatch(/^@e\d+$/);

		const text = await executeCommand({
			id: "fr-ref-2",
			action: "getText",
			target: { ref },
		});
		expect(text.success).toBe(true);
		expect(text.data).toBe("Save");
	});

	it("errors when the frame does not exist", async () => {
		const result = await executeCommand({
			id: "fr-4",
			action: "getText",
			target: { selector: "body" },
			options: { frame: "#missing" },
		});

		expect(result.success).toBe(false);
		expect(result.error).toBe("frame #missing not found");
	});

	it("errors when the match is not an iframe", async () => {
		document.body.innerHTML = '<div id="not-a-frame"></div>';

		const result = await executeCommand({
			id: "fr-5",
			action: "getText",
			target: { selector: "body" },
			options: { frame: "#not-a-frame" },
		});

		expect(result.success).toBe(false);
		expect(result.error).toBe("frame #not-a-frame is not an <iframe>");
	});

	it("errors when the frame is cross-origin", async () => {
		const { frame } = makeFrame("frame-x");
		Object.defineProperty(frame, "contentDocument", {
			configurable: true,
			get: () => null,
		});

		const result = await executeCommand({
			id: "fr-6",
			action: "getText",
			target: { selector: "body" },
			options: { frame: "#frame-x" },
		});

		expect(result.success).toBe(false);
		expect(result.error).toBe(
			"frame #frame-x is cross-origin; open its URL in a tab",
		);
	});

	it("rejects options.frame on an action that cannot honour it", async () => {
		makeFrame("frame-y");

		const result = await executeCommand({
			id: "fr-7",
			action: "extractTable",
			target: { selector: "table" },
			options: { frame: "#frame-y" },
		});

		expect(result.success).toBe(false);
		expect(result.error).toBe(
			'action "extractTable" does not support options.frame',
		);
	});
});

describe("snapshot and findAll geometry (DOM)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	type Node = { role: string; box?: unknown; children?: Node[] };
	function findNode(node: Node, role: string): Node | undefined {
		if (node.role === role) return node;
		for (const child of node.children ?? []) {
			const found = findNode(child, role);
			if (found) return found;
		}
		return undefined;
	}

	it("gives a snapshot node its viewport box and the root its viewport size", async () => {
		document.body.innerHTML = '<button id="geo-btn">Go</button>';

		const result = await executeCommand({ id: "geo-1", action: "snapshot" });

		expect(result.success).toBe(true);
		const tree = result.data as Node & {
			viewport?: { w: number; h: number };
		};
		expect(tree.viewport).toEqual({
			w: window.innerWidth,
			h: window.innerHeight,
		});
		// domSetup gives every element a 100x50 box at the origin.
		expect(findNode(tree, "button")?.box).toEqual({
			x: 0,
			y: 0,
			w: 100,
			h: 50,
		});
	});

	it("reports the viewport scroll offset on the snapshot root", async () => {
		document.body.innerHTML = '<button id="scroll-btn">Go</button>';
		const original = Object.getOwnPropertyDescriptor(window, "scrollY");
		Object.defineProperty(window, "scrollY", {
			value: 120,
			configurable: true,
		});
		try {
			const result = await executeCommand({
				id: "geo-scroll",
				action: "snapshot",
			});
			const tree = result.data as Node & { scroll?: { x: number; y: number } };
			expect(tree.scroll).toEqual({ x: window.scrollX, y: 120 });
		} finally {
			if (original) Object.defineProperty(window, "scrollY", original);
		}
	});

	it("omits box when the element has no layout box", async () => {
		document.body.innerHTML = '<button id="zero-btn">Go</button>';
		const button = document.querySelector("#zero-btn") as HTMLElement;
		button.getBoundingClientRect = () =>
			({ left: 0, top: 0, width: 0, height: 0 }) as DOMRect;

		const result = await executeCommand({ id: "geo-2", action: "snapshot" });

		const node = findNode(result.data as Node, "button");
		expect(node).toBeDefined();
		expect("box" in (node as Node)).toBe(false);
	});

	it("gives findAll entries a box", async () => {
		document.body.innerHTML = '<a id="geo-link" href="/x">Next</a>';

		const result = await executeCommand({
			id: "geo-3",
			action: "findAll",
			target: { selector: "#geo-link" },
		});

		const entries = result.data as { box?: unknown }[];
		expect(entries).toHaveLength(1);
		expect(entries[0].box).toEqual({ x: 0, y: 0, w: 100, h: 50 });
	});
});

describe("hover at viewport coordinates (DOM)", () => {
	afterEach(() => {
		document.body.innerHTML = "";
		delete (document as unknown as Record<string, unknown>).elementFromPoint;
	});

	it("dispatches mouseover and mouseenter on the element under the point", async () => {
		const el = document.createElement("div");
		el.id = "hover-target";
		document.body.appendChild(el);
		const seen: string[] = [];
		el.addEventListener("mouseover", () => seen.push("mouseover"));
		el.addEventListener("mouseenter", () => seen.push("mouseenter"));
		(document as unknown as Record<string, unknown>).elementFromPoint = () =>
			el;

		const result = await executeCommand({
			id: "hx1",
			action: "hover",
			target: { x: 10, y: 20 },
		});

		expect(result.success).toBe(true);
		expect(seen).toContain("mouseover");
		expect(seen).toContain("mouseenter");
	});

	it("fails naming the coordinate when nothing is under the point", async () => {
		(document as unknown as Record<string, unknown>).elementFromPoint = () =>
			null;

		const result = await executeCommand({
			id: "hx2",
			action: "hover",
			target: { x: 5, y: 6 },
		});

		expect(result.success).toBe(false);
		expect(result.error).toMatch(/nothing at \(5, 6\)/);
	});
});

describe("sequence (compound action)", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	afterEach(() => {
		document.body.innerHTML = "";
	});

	function makeInput(id: string): HTMLInputElement {
		const el = document.createElement("input");
		el.id = id;
		document.body.appendChild(el);
		return el;
	}

	it("runs every step in order", async () => {
		const a = makeInput("seq-a");
		const b = makeInput("seq-b");

		const result = await executeCommand({
			id: "sq1",
			action: "sequence",
			options: {
				steps: [
					{ action: "fill", target: { selector: "#seq-a" }, value: "one" },
					{ action: "fill", target: { selector: "#seq-b" }, value: "two" },
				],
			},
		});

		expect(result.success).toBe(true);
		expect(a.value).toBe("one");
		expect(b.value).toBe("two");
		const data = result.data as { steps: { step: number }[] };
		expect(data.steps.map((entry) => entry.step)).toEqual([1, 2]);
	});

	it("lets a later step act on DOM a previous step created (the menu case)", async () => {
		const trigger = document.createElement("button");
		trigger.id = "seq-open";
		document.body.appendChild(trigger);
		let clicked = false;
		trigger.addEventListener("click", () => {
			const item = document.createElement("div");
			item.setAttribute("role", "menuitem");
			item.textContent = "Move";
			item.addEventListener("click", () => {
				clicked = true;
			});
			document.body.appendChild(item);
		});

		const result = await executeCommand({
			id: "sq2",
			action: "sequence",
			options: {
				steps: [
					{ action: "click", target: { selector: "#seq-open" } },
					{ action: "click", target: { role: "menuitem" } },
				],
			},
		});

		expect(result.success).toBe(true);
		expect(clicked).toBe(true);
	});

	it("stops at the first failing step, names it, and runs nothing after", async () => {
		const first = makeInput("seq-c");
		const third = makeInput("seq-d");

		const result = await executeCommand({
			id: "sq3",
			action: "sequence",
			options: {
				steps: [
					{ action: "fill", target: { selector: "#seq-c" }, value: "ok" },
					{
						action: "fill",
						target: { selector: "#seq-missing" },
						value: "x",
						options: { timeout: 100 },
					},
					{ action: "fill", target: { selector: "#seq-d" }, value: "never" },
				],
			},
		});

		expect(result.success).toBe(false);
		expect(result.error).toMatch(/step 2 \(fill\) failed/);
		expect(first.value).toBe("ok");
		expect(third.value).toBe("");
	});

	it("rejects an empty step list", async () => {
		const result = await executeCommand({
			id: "sq4",
			action: "sequence",
			options: { steps: [] },
		});

		expect(result.success).toBe(false);
		expect(result.error).toMatch(/non-empty/);
	});

	it("rejects a step without an action", async () => {
		const result = await executeCommand({
			id: "sq5",
			action: "sequence",
			options: { steps: [{ target: { selector: "#seq-x" } }] },
		});

		expect(result.success).toBe(false);
		expect(result.error).toMatch(/step 1 has no "action"/);
	});

	it("honours waitMs before a step", async () => {
		const target = makeInput("seq-e");
		const start = Date.now();

		const result = await executeCommand({
			id: "sq6",
			action: "sequence",
			options: {
				steps: [
					{
						action: "fill",
						target: { selector: "#seq-e" },
						value: "waited",
						waitMs: 60,
					},
				],
			},
		});

		expect(result.success).toBe(true);
		expect(target.value).toBe("waited");
		expect(Date.now() - start).toBeGreaterThanOrEqual(50);
	});
});
