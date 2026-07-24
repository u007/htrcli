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
});
