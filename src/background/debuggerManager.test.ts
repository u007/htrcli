import { beforeEach, describe, expect, it } from "bun:test";
import {
	__setDebuggerImplForTests,
	attachShared,
	detachShared,
	onDebuggerEvent,
} from "./debuggerManager";

interface FakeDebugger {
	attachCalls: number;
	detachCalls: number;
	emit: (source: { tabId?: number }, method: string, params: unknown) => void;
}

function installFakeDebugger(): FakeDebugger {
	let attachCalls = 0;
	let detachCalls = 0;
	let handler:
		| ((source: { tabId?: number }, method: string, params: unknown) => void)
		| null = null;
	__setDebuggerImplForTests({
		attach: async () => {
			attachCalls++;
		},
		detach: async () => {
			detachCalls++;
		},
		sendCommand: async () => ({}),
		onEvent: {
			addListener: (cb) => {
				handler = cb;
			},
			removeListener: () => {
				handler = null;
			},
		},
	});
	return {
		get attachCalls() {
			return attachCalls;
		},
		get detachCalls() {
			return detachCalls;
		},
		emit: (source, method, params) => handler?.(source, method, params),
	};
}

describe("debuggerManager", () => {
	beforeEach(() => {
		__setDebuggerImplForTests(null);
	});

	it("attaches once and detaches once across balanced refcounted calls", async () => {
		const fake = installFakeDebugger();
		await attachShared(7);
		await attachShared(7);
		expect(fake.attachCalls).toBe(1);
		await detachShared(7);
		expect(fake.detachCalls).toBe(0); // still one holder
		await detachShared(7);
		expect(fake.detachCalls).toBe(1);
	});

	it("delivers debugger events only to subscribers until they unsubscribe", async () => {
		const fake = installFakeDebugger();
		await attachShared(7);
		const received: string[] = [];
		const unsub = onDebuggerEvent((source, method) => {
			if (source.tabId === 7) received.push(method);
		});
		fake.emit({ tabId: 7 }, "Network.responseReceived", {});
		unsub();
		fake.emit({ tabId: 7 }, "Network.loadingFinished", {});
		expect(received).toEqual(["Network.responseReceived"]);
	});

	it("does not throw 'Already attached' when dispatching during an open capture", async () => {
		const fake = installFakeDebugger();
		await attachShared(42);
		expect(fake.attachCalls).toBe(1);
		// A second attachShared (simulating an open capture + dispatch)
		// must increment the refcount, not throw.
		await attachShared(42);
		expect(fake.attachCalls).toBe(1); // refcounted: still only one real attach
		await detachShared(42);
		expect(fake.detachCalls).toBe(0); // still one holder (the capture)
		await detachShared(42);
		expect(fake.detachCalls).toBe(1);
	});
});

describe("attachShared concurrency", () => {
	beforeEach(() => {
		__setDebuggerImplForTests(null);
	});

	function installSlowFake(attachDelayMs: number) {
		const state = { attachCalls: 0, detachCalls: 0 };
		__setDebuggerImplForTests({
			attach: async () => {
				state.attachCalls++;
				await new Promise((r) => setTimeout(r, attachDelayMs));
			},
			detach: async () => {
				state.detachCalls++;
			},
			sendCommand: async () => ({}),
			onEvent: {
				addListener: () => undefined,
				removeListener: () => undefined,
			},
		});
		return state;
	}

	it("coalesces two overlapping attaches into ONE real attach", async () => {
		// Without coalescing both callers read refcount 0 and both call
		// chrome.debugger.attach; the loser gets "Another debugger is already
		// attached".
		const state = installSlowFake(10);
		await Promise.all([attachShared(7), attachShared(7)]);
		expect(state.attachCalls).toBe(1);
		// Both holders are registered.
		await detachShared(7);
		expect(state.detachCalls).toBe(0);
		await detachShared(7);
		expect(state.detachCalls).toBe(1);
	});

	it("a failed attach leaves no hold behind", async () => {
		const state = { attachCalls: 0, detachCalls: 0, fail: true };
		__setDebuggerImplForTests({
			attach: async () => {
				state.attachCalls++;
				if (state.fail) throw new Error("Another debugger is already attached");
			},
			detach: async () => {
				state.detachCalls++;
			},
			sendCommand: async () => ({}),
			onEvent: {
				addListener: () => undefined,
				removeListener: () => undefined,
			},
		});

		await expect(attachShared(7)).rejects.toThrow(/already attached/);

		// The refcount must have been rolled back, so the compensating detach a
		// caller performs in its catch block is a NO-OP rather than a release
		// of somebody else's hold.
		await detachShared(7);
		expect(state.detachCalls).toBe(0);

		// And a later attach must still perform a real attach.
		state.fail = false;
		await attachShared(7);
		expect(state.attachCalls).toBe(2);
		await detachShared(7);
		expect(state.detachCalls).toBe(1);
	});
});
