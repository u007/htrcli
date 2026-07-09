import { beforeEach, describe, expect, it } from "bun:test";
import {
	getConnectionMode,
	isPermanentError,
	startNativeHost,
} from "./nativeHost";

// ─── Fake chrome + native port ───────────────────────────────────────
// Exercises the connection-confirmation state machine (portConfirmed /
// confirmConnected / the single-port guard) without a real browser.

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
	// Simulate the daemon sending a native message to this port.
	emit(m: unknown) {
		this.msgListeners.forEach((l) => {
			l(m);
		});
	}
}

let currentPort: FakePort | null = null;
const createdPorts: FakePort[] = [];

function getPort(): FakePort {
	if (!currentPort) throw new Error("no native port was opened");
	return currentPort;
}

function installFakeChrome() {
	const fakeChrome = {
		runtime: {
			connectNative: (_host: string) => {
				const p = new FakePort();
				currentPort = p;
				createdPorts.push(p);
				return p;
			},
			lastError: undefined as { message?: string } | undefined,
		},
		tabs: {
			query: (_q: unknown, cb: (t: unknown[]) => void) => cb([]),
		},
	};
	(globalThis as { chrome?: unknown }).chrome = fakeChrome;
}

describe("isPermanentError", () => {
	it("treats a missing/unregistered host as permanent (no reconnect)", () => {
		expect(isPermanentError("Specified native messaging host not found.")).toBe(
			true,
		);
		expect(isPermanentError("No such native application com.x.host")).toBe(
			true,
		);
		expect(isPermanentError("host not installed")).toBe(true);
		expect(
			isPermanentError(
				"Access to the specified native messaging host is forbidden.",
			),
		).toBe(true);
	});

	it("treats 'Native host has exited' as transient so it reconnects", () => {
		// Chrome reports this when the relay quits because the daemon socket is
		// down. Retrying with backoff must recover once the daemon starts.
		expect(isPermanentError("Native host has exited.")).toBe(false);
	});

	it("treats an unknown/relay-death disconnect as transient", () => {
		expect(isPermanentError("unknown")).toBe(false);
		expect(
			isPermanentError(
				"Error when communicating with the native messaging host.",
			),
		).toBe(false);
	});
});

describe("connection confirmation state machine", () => {
	beforeEach(() => {
		createdPorts.length = 0;
		currentPort = null;
		installFakeChrome();
	});

	it("stays disconnected until the daemon sends its greeting ping", () => {
		startNativeHost();
		// connectNative succeeded but we have no proof the daemon is reachable.
		expect(getConnectionMode()).toBe("disconnected");

		const port = getPort();
		port.emit({ type: "ping" });

		expect(getConnectionMode()).toBe("native");
		// The extension must reply with a heartbeat so the daemon doesn't reap it.
		expect(
			port.messages.some((m) => (m as { type?: string }).type === "heartbeat"),
		).toBe(true);
	});

	it("does not confirm on a relay 'error' message", () => {
		startNativeHost();
		const port = getPort();
		port.emit({ type: "error", error: "daemon socket down" });
		expect(getConnectionMode()).toBe("disconnected");
	});

	it("ignores messages from a superseded (stale) port after reconnect", () => {
		startNativeHost();
		const portA = getPort();
		portA.emit({ type: "ping" });
		expect(getConnectionMode()).toBe("native");

		// A reconnect must not leave two live ports; the old one is closed and
		// its traffic must be ignored so it can't confirm or command the new one.
		startNativeHost();
		const portB = getPort();
		expect(portB).not.toBe(portA);

		portA.emit({ type: "ping" });
		expect(getConnectionMode()).toBe("disconnected");

		portB.emit({ type: "ping" });
		expect(getConnectionMode()).toBe("native");
	});
});
