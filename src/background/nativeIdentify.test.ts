import { beforeEach, describe, expect, it } from "bun:test";
import { getBrowserType, startNativeHost } from "./nativeHost";

// The daemon resolves tab-less commands (session recording) by browser. It
// learns which browser a relay is by an `identify` message the extension sends
// once per connection — after the daemon's greeting confirms the chain, not on
// port open, because connectNative succeeds even when the daemon is down.

class FakePort {
	messages: unknown[] = [];
	private msgListeners: Array<(m: unknown) => void> = [];
	private discListeners: Array<(m: unknown) => void> = [];
	onMessage = {
		addListener: (l: (m: unknown) => void) => {
			this.msgListeners.push(l);
		},
	};
	onDisconnect = {
		addListener: (l: (m: unknown) => void) => {
			this.discListeners.push(l);
		},
	};
	postMessage(m: object) {
		this.messages.push(m);
	}
	disconnect() {
		this.discListeners.forEach((l) => {
			l({});
		});
	}
	emit(m: unknown) {
		this.msgListeners.forEach((l) => {
			l(m);
		});
	}
}

let currentPort: FakePort | null = null;

function installFakeChrome() {
	const fakeChrome = {
		runtime: {
			connectNative: () => {
				currentPort = new FakePort();
				return currentPort;
			},
			lastError: undefined as { message?: string } | undefined,
		},
		storage: {
			local: {
				get: (_k: string, cb: (v: unknown) => void) => cb({}),
				set: (_k: string, _v: unknown, cb?: () => void) => cb?.(),
			},
		},
		tabs: {
			query: (_q: unknown, cb: (t: unknown[]) => void) => cb([]),
		},
	};
	(globalThis as unknown as { chrome: unknown }).chrome = fakeChrome;
}

function identifies(): Array<Record<string, unknown>> {
	if (!currentPort) throw new Error("no native port was opened");
	return currentPort.messages.filter(
		(m): m is Record<string, unknown> =>
			typeof m === "object" &&
			m !== null &&
			(m as { type?: string }).type === "identify",
	);
}

describe("native host identify", () => {
	beforeEach(() => {
		currentPort = null;
		installFakeChrome();
	});

	// The daemon greeting is what confirms relay↔daemon, so identify must not
	// fire on port open: at that point the daemon may not even be running.
	it("does not identify before the daemon confirms the connection", () => {
		startNativeHost();
		if (!currentPort) throw new Error("no native port was opened");
		expect(identifies()).toHaveLength(0);
	});

	it("identifies with the browser getBrowserType reports once the daemon confirms", () => {
		startNativeHost();
		if (!currentPort) throw new Error("no native port was opened");
		currentPort.emit({ type: "ping", id: "greeting-1" });

		const sent = identifies();
		expect(sent).toHaveLength(1);
		expect(sent[0].browser).toBe(getBrowserType());
	});

	// confirmConnected is guarded by portConfirmed, but that flag is the thing
	// that could regress; assert the observable contract instead of trusting it.
	it("sends exactly one identify no matter how many messages arrive", () => {
		startNativeHost();
		if (!currentPort) throw new Error("no native port was opened");
		for (const id of ["greeting-1", "greeting-2", "greeting-3"]) {
			currentPort.emit({ type: "ping", id });
		}
		expect(identifies()).toHaveLength(1);
	});

	// Each reconnect creates a NEW daemon-side connection that starts with no
	// identity, so the announcement has to be repeated — not remembered.
	it("identifies again after a reconnect", () => {
		startNativeHost();
		if (!currentPort) throw new Error("no native port was opened");
		currentPort.emit({ type: "ping", id: "greeting-1" });
		const first = identifies();
		expect(first).toHaveLength(1);

		// Drop the port the way a dead relay would, then let the reconnect land.
		const firstPort = currentPort;
		firstPort.disconnect();
		startNativeHost();
		if (!currentPort) throw new Error("no native port was opened");
		expect(currentPort).not.toBe(firstPort);
		currentPort.emit({ type: "ping", id: "greeting-2" });

		// The new port carries its own identify; the dead one keeps the old.
		expect(identifies()).toHaveLength(1);
		expect(identifies()[0].browser).toBe(getBrowserType());
	});
});
