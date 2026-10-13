import { beforeEach, describe, expect, it } from "bun:test";
import { getBrowserType } from "./browserType";
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

// ─── Tab-less openTab / closeTab (htrcli tabs new / tabs close) ─────
// These arrive on the background route with tabId 0 and must be answered by
// the background worker, not forwarded to a content script.

describe("tab-less openTab / closeTab", () => {
	const flush = () => new Promise((resolve) => setTimeout(resolve, 0));
	let tabsCreated: Array<{ url: string; active: boolean }> = [];
	let tabsRemoved: number[] = [];
	let updatedListeners: Array<
		(tid: number, info: { status?: string }) => void
	> = [];

	type CommandResultPayload = {
		id: string;
		success: boolean;
		data?: unknown;
		error?: string;
	};

	function resultFor(id: string): CommandResultPayload | undefined {
		const found = getPort().messages.find(
			(m) =>
				(m as { type?: string }).type === "command_result" &&
				(m as { payload?: { id?: string } }).payload?.id === id,
		) as { payload: CommandResultPayload } | undefined;
		return found?.payload;
	}

	beforeEach(() => {
		createdPorts.length = 0;
		currentPort = null;
		installFakeChrome();
		tabsCreated = [];
		tabsRemoved = [];
		updatedListeners = [];
		const fake = (globalThis as { chrome?: { tabs?: unknown } }).chrome;
		if (!fake) throw new Error("fake chrome not installed");
		fake.tabs = {
			query: (_q: unknown, cb: (t: unknown[]) => void) => cb([]),
			create: async (props: { url: string; active: boolean }) => {
				tabsCreated.push(props);
				return { id: 42 };
			},
			remove: async (id: number) => {
				tabsRemoved.push(id);
			},
			onUpdated: {
				addListener: (l: (tid: number, info: { status?: string }) => void) => {
					updatedListeners.push(l);
				},
				removeListener: (
					l: (tid: number, info: { status?: string }) => void,
				) => {
					updatedListeners = updatedListeners.filter((x) => x !== l);
				},
			},
		};
		startNativeHost();
		getPort().emit({ type: "ping" });
	});

	it("openTab with tabId 0 opens an inactive tab and reports the browser", async () => {
		getPort().emit({
			type: "command",
			tabId: 0,
			payload: {
				id: "t1",
				action: "openTab",
				options: { url: "https://example.com" },
			},
		});
		await flush();
		for (const listener of updatedListeners)
			listener(42, { status: "complete" });
		await flush();

		expect(tabsCreated).toEqual([
			{ url: "https://example.com", active: false },
		]);
		expect(resultFor("t1")).toEqual({
			id: "t1",
			success: true,
			data: { tabId: 42, browser: getBrowserType() },
		});
	});

	it("closeTab with tabId 0 removes the tab and reports the browser", async () => {
		getPort().emit({
			type: "command",
			tabId: 0,
			payload: { id: "t2", action: "closeTab", options: { tabId: 42 } },
		});
		await flush();

		expect(tabsRemoved).toEqual([42]);
		expect(resultFor("t2")?.success).toBe(true);
		expect(resultFor("t2")?.data).toEqual({ browser: getBrowserType() });
	});

	it("closeTab without a positive tabId fails loudly and removes nothing", async () => {
		getPort().emit({
			type: "command",
			tabId: 0,
			payload: { id: "t3", action: "closeTab", options: { tabId: 0 } },
		});
		await flush();

		expect(tabsRemoved).toEqual([]);
		expect(resultFor("t3")?.success).toBe(false);
		expect(resultFor("t3")?.error).toContain("positive options.tabId");
	});

	it("openTab without a url fails loudly and opens nothing", async () => {
		getPort().emit({
			type: "command",
			tabId: 0,
			payload: { id: "t4", action: "openTab", options: {} },
		});
		await flush();

		expect(tabsCreated).toEqual([]);
		expect(resultFor("t4")?.success).toBe(false);
		expect(resultFor("t4")?.error).toContain("options.url");
	});
});
