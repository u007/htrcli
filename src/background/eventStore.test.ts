import { beforeEach, describe, expect, it } from "bun:test";
import {
	__resetEventStoreForTests,
	flushPending,
	recordConsoleEntry,
	recordNetworkEntry,
} from "./eventStore";

function installStorageMock() {
	const store: Record<string, unknown> = {};
	const scope = globalThis as typeof globalThis & { chrome?: unknown };
	scope.chrome = {
		storage: {
			session: {
				get: ((
					keyOrKeys?: string | string[] | Record<string, unknown> | null,
					callback?: (items: Record<string, unknown>) => void,
				) => {
					let result: Record<string, unknown>;
					if (keyOrKeys == null) {
						result = { ...store };
					} else if (typeof keyOrKeys === "string") {
						result = { [keyOrKeys]: store[keyOrKeys] };
					} else if (Array.isArray(keyOrKeys)) {
						result = Object.fromEntries(keyOrKeys.map((k) => [k, store[k]]));
					} else {
						result = { ...store };
					}
					if (callback) {
						callback(result);
						return;
					}
					return Promise.resolve(result);
				}) as typeof chrome.storage.session.get,
				set: ((values: Record<string, unknown>, callback?: () => void) => {
					Object.assign(store, values);
					callback?.();
					return Promise.resolve();
				}) as typeof chrome.storage.session.set,
			},
		},
	} as unknown as typeof chrome;
	return store;
}

describe("eventStore console capture", () => {
	beforeEach(() => {
		installStorageMock();
		__resetEventStoreForTests();
	});

	it("assigns increasing seq numbers per tab", async () => {
		await recordConsoleEntry(1, { level: "log", args: ["a"] });
		await recordConsoleEntry(1, { level: "log", args: ["b"] });
		const posted: unknown[] = [];
		await flushPending(async (tabId, kind, entries) => {
			posted.push({ tabId, kind, entries });
			return true;
		});
		const call = posted[0] as { entries: { seq: number }[] };
		expect(call.entries.map((entry) => entry.seq)).toEqual([1, 2]);
	});

	it("caps at 500 entries per bucket", async () => {
		for (let i = 0; i < 501; i++) {
			await recordConsoleEntry(1, { level: "log", args: [String(i)] });
		}
		let capturedEntries: { seq: number }[] = [];
		await flushPending(async (_tabId, _kind, entries) => {
			capturedEntries = entries as { seq: number }[];
			return true;
		});
		expect(capturedEntries.length).toBe(500);
		expect(capturedEntries[0].seq).toBe(2);
	});

	it("retries a failed POST instead of dropping entries", async () => {
		await recordConsoleEntry(1, { level: "log", args: ["retry-me"] });
		let attempts = 0;
		await flushPending(async () => {
			attempts += 1;
			return false;
		});
		let secondAttemptEntries = 0;
		await flushPending(async (_tabId, _kind, entries) => {
			secondAttemptEntries = entries.length;
			return true;
		});
		expect(attempts).toBe(1);
		expect(secondAttemptEntries).toBe(1);
	});

	it("records network entries in a separate bucket with their own seq", async () => {
		await recordNetworkEntry(1, {
			requestId: "req-1",
			url: "https://example.com/api/users",
			method: "GET",
			status: 200,
			durationMs: 42,
		});
		await recordNetworkEntry(1, {
			requestId: "req-2",
			url: "https://example.com/api/orders",
			method: "POST",
			status: 500,
			durationMs: 7,
		});

		const posted: {
			kind: string;
			entries: { seq: number; data: { url: string } }[];
		}[] = [];
		await flushPending(async (_tabId, kind, entries) => {
			posted.push({
				kind,
				entries: entries as { seq: number; data: { url: string } }[],
			});
			return true;
		});

		const networkBatch = posted.find((p) => p.kind === "network");
		expect(networkBatch).toBeDefined();
		expect(networkBatch?.entries.map((e) => e.seq)).toEqual([1, 2]);
		expect(networkBatch?.entries[0].data.url).toBe(
			"https://example.com/api/users",
		);
	});

	it("keeps console and network seq counters independent per tab", async () => {
		await recordConsoleEntry(1, { level: "log", args: ["a"] });
		await recordNetworkEntry(1, {
			requestId: "req-1",
			url: "https://example.com/x",
			method: "GET",
		});

		const byKind: Record<string, number[]> = {};
		await flushPending(async (_tabId, kind, entries) => {
			byKind[kind] = (entries as { seq: number }[]).map((e) => e.seq);
			return true;
		});
		expect(byKind.console).toEqual([1]);
		expect(byKind.network).toEqual([1]);
	});
});
