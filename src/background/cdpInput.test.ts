import { describe, expect, it } from "bun:test";
import {
	CDP_INPUT_ACTIONS,
	ContentScriptNotReadyError,
	dispatchCdpClick,
	dispatchCdpInput,
	dispatchCdpKey,
	dispatchCdpType,
} from "./cdpInput";

/**
 * Test seam: a CDP `send` stub that records every dispatched event so we can
 * assert the exact event sequence without a real debugger attached.
 */
function recordingSend() {
	const calls: { method: string; params: Record<string, unknown> }[] = [];
	const send = async (method: string, params: Record<string, unknown>) => {
		calls.push({ method, params });
		return undefined;
	};
	return { send, calls };
}

const coordsPrepare = async () => ({ x: 50, y: 25 });
const focusPrepare = async () => ({ focused: true });

function clickCmd(
	action: "click" | "dblclick" | "rightclick",
): Parameters<typeof dispatchCdpClick>[1] {
	return { id: "c1", action, target: { selector: "#b" } };
}

describe("CDP input dispatch — event sequence construction", () => {
	it("click dispatches mousePressed + mouseReleased with button 'left', clickCount 1", async () => {
		const { send, calls } = recordingSend();
		const result = await dispatchCdpClick(1, clickCmd("click"), {
			send,
			prepare: coordsPrepare,
		});
		expect(result.success).toBe(true);

		expect(calls.map((c) => c.method)).toEqual([
			"Input.dispatchMouseEvent",
			"Input.dispatchMouseEvent",
		]);
		expect(calls[0].params.type).toBe("mousePressed");
		expect(calls[0].params.button).toBe("left");
		expect(calls[0].params.clickCount).toBe(1);
		expect(calls[1].params.type).toBe("mouseReleased");
		expect(calls[1].params.button).toBe("left");
		expect(calls[1].params.clickCount).toBe(1);
	});

	it("dblclick uses clickCount 2 on both events", async () => {
		const { send, calls } = recordingSend();
		await dispatchCdpClick(1, clickCmd("dblclick"), {
			send,
			prepare: coordsPrepare,
		});
		expect(calls[0].params.type).toBe("mousePressed");
		expect(calls[0].params.clickCount).toBe(2);
		expect(calls[1].params.type).toBe("mouseReleased");
		expect(calls[1].params.clickCount).toBe(2);
	});

	it("rightclick uses button 'right'", async () => {
		const { send, calls } = recordingSend();
		await dispatchCdpClick(1, clickCmd("rightclick"), {
			send,
			prepare: coordsPrepare,
		});
		expect(calls[0].params.button).toBe("right");
		expect(calls[1].params.button).toBe("right");
	});

	it("pressKey Enter dispatches keyDown with text '\\r' then keyUp", async () => {
		const { send, calls } = recordingSend();
		await dispatchCdpKey(
			1,
			{
				id: "k1",
				action: "pressKey",
				target: { selector: "#i" },
				value: "Enter",
			},
			{ send, prepare: focusPrepare },
		);
		expect(calls.map((c) => c.method)).toEqual([
			"Input.dispatchKeyEvent",
			"Input.dispatchKeyEvent",
		]);
		expect(calls[0].params.type).toBe("keyDown");
		expect(calls[0].params.key).toBe("Enter");
		expect(calls[0].params.code).toBe("Enter");
		expect(calls[0].params.text).toBe("\r");
		expect(calls[1].params.type).toBe("keyUp");
		expect(calls[1].params.text).toBeUndefined();
	});

	it("type dispatches a single Input.insertText call with the whole string", async () => {
		const { send, calls } = recordingSend();
		await dispatchCdpType(
			1,
			{ id: "t1", action: "type", target: { selector: "#i" }, value: "hello" },
			{ send, prepare: focusPrepare },
		);
		expect(calls).toHaveLength(1);
		expect(calls[0].method).toBe("Input.insertText");
		expect(calls[0].params.text).toBe("hello");
	});

	it("dispatchCdpInput routes each action to the correct dispatcher", async () => {
		const { send, calls } = recordingSend();

		await dispatchCdpInput(1, clickCmd("click"), {
			send,
			prepare: coordsPrepare,
		});
		expect(calls[0].params.type).toBe("mousePressed");

		calls.length = 0;
		await dispatchCdpInput(
			1,
			{ id: "k2", action: "pressKey", target: { selector: "#i" }, value: "a" },
			{ send, prepare: focusPrepare },
		);
		expect(calls[0].method).toBe("Input.dispatchKeyEvent");

		calls.length = 0;
		await dispatchCdpInput(
			1,
			{ id: "t2", action: "type", target: { selector: "#i" }, value: "x" },
			{ send, prepare: focusPrepare },
		);
		expect(calls[0].method).toBe("Input.insertText");
	});

	it("exposes the set of CDP-input actions", () => {
		expect([...CDP_INPUT_ACTIONS].sort()).toEqual([
			"click",
			"dblclick",
			"pressKey",
			"rightclick",
			"type",
		]);
	});
});

describe("CDP input dispatch — error handling", () => {
	it("throws when prepareClick returns no coordinates", async () => {
		const { send } = recordingSend();
		const badPrepare = async () => ({ focused: true });
		await expect(
			dispatchCdpClick(1, clickCmd("click"), { send, prepare: badPrepare }),
		).rejects.toThrow(/did not return viewport coordinates/i);
	});

	it("propagates a not-ready content script as a retryable error", async () => {
		const { send } = recordingSend();
		const notReady = async () => {
			throw new ContentScriptNotReadyError();
		};
		await expect(
			dispatchCdpClick(1, clickCmd("click"), { send, prepare: notReady }),
		).rejects.toBeInstanceOf(ContentScriptNotReadyError);
	});
});
