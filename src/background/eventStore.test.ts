import { beforeEach, describe, expect, it } from "bun:test";
import {
	__resetEventStoreForTests,
	drainTabBuckets,
	flushPending,
	recordConsoleEntry,
	recordDialogEntry,
	recordNetworkEntry,
	resetForResync,
} from "./eventStore";

function installStorageMock(beforeSet?: () => Promise<void> | void) {
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
				set: (async (
					values: Record<string, unknown>,
					callback?: () => void,
				) => {
					Object.assign(store, values);
					callback?.();
					await beforeSet?.();
				}) as typeof chrome.storage.session.set,
			},
		},
	} as unknown as typeof chrome;
	return store;
}

describe("eventStore console capture", () => {
	let store: Record<string, unknown>;

	beforeEach(() => {
		store = installStorageMock();
		__resetEventStoreForTests();
	});

	function bucketKeys(): string[] {
		const persisted = store["htrncontrol:event-store"] as
			| { buckets: Record<string, unknown> }
			| undefined;
		if (!persisted) throw new Error("event store was never persisted");
		return Object.keys(persisted.buckets).sort();
	}

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

	it("does not resolve until the accepted event is persisted", async () => {
		let setCalled = false;
		let releaseSave!: () => void;
		const saveGate = new Promise<void>((resolve) => {
			releaseSave = resolve;
		});
		store = installStorageMock(async () => {
			setCalled = true;
			await saveGate;
		});
		__resetEventStoreForTests();

		let recordResolved = false;
		const recordPromise = recordConsoleEntry(1, {
			level: "log",
			args: ["durable"],
		});
		recordPromise.then(() => {
			recordResolved = true;
		});

		await Promise.resolve();
		await Promise.resolve();
		const resolvedBeforeSave = recordResolved;
		releaseSave();
		await recordPromise;

		expect(setCalled).toBe(true);
		expect(resolvedBeforeSave).toBe(false);
		expect(
			(
				(
					store["htrncontrol:event-store"] as {
						buckets: Record<string, unknown>;
					}
				).buckets["1:console"] as { entries: unknown[] }
			).entries,
		).toHaveLength(1);
	});

	it("persists concurrent records with contiguous sequence numbers", async () => {
		await Promise.all(
			Array.from({ length: 20 }, (_, index) =>
				recordConsoleEntry(1, { level: "log", args: [String(index)] }),
			),
		);

		const bucket = (
			store["htrncontrol:event-store"] as { buckets: Record<string, unknown> }
		).buckets["1:console"] as {
			entries: { seq: number }[];
		};
		expect(bucket.entries.map((entry) => entry.seq)).toEqual(
			Array.from({ length: 20 }, (_, index) => index + 1),
		);
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

	it("records dialog entries in their own bucket", async () => {
		await recordDialogEntry(1, {
			dialogType: "confirm",
			message: "Delete this item?",
			resolvedAction: "accept",
		});

		const posted: { kind: string; entries: { data: { message: string } }[] }[] =
			[];
		await flushPending(async (_tabId, kind, entries) => {
			posted.push({
				kind,
				entries: entries as { data: { message: string } }[],
			});
			return true;
		});

		const dialogBatch = posted.find((p) => p.kind === "dialog");
		expect(dialogBatch).toBeDefined();
		expect(dialogBatch?.entries[0].data.message).toBe("Delete this item?");
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

	it("re-flushes all entries after resetForResync (daemon restart durability)", async () => {
		// Record 3 events and flush successfully.
		await recordConsoleEntry(1, { level: "log", args: ["a"] });
		await recordConsoleEntry(1, { level: "log", args: ["b"] });
		await recordConsoleEntry(1, { level: "log", args: ["c"] });
		let firstFlush: number[] = [];
		await flushPending(async (_tabId, _kind, entries) => {
			firstFlush = (entries as { seq: number }[]).map((e) => e.seq);
			return true;
		});
		expect(firstFlush).toEqual([1, 2, 3]);

		// After flush, entries stay in the buffer (watermark advances).
		// A second flush should send nothing new.
		let secondFlush: number[] = [];
		await flushPending(async (_tabId, _kind, entries) => {
			secondFlush = (entries as { seq: number }[]).map((e) => e.seq);
			return true;
		});
		expect(secondFlush).toEqual([]);

		// Simulate daemon restart: resetForResync resets the watermark.
		await resetForResync();

		// Now flush replays ALL entries, not just new ones.
		let restartFlush: number[] = [];
		await flushPending(async (_tabId, _kind, entries) => {
			restartFlush = (entries as { seq: number }[]).map((e) => e.seq);
			return true;
		});
		expect(restartFlush).toEqual([1, 2, 3]);
	});

	it("drains a closed tab's buckets without touching other tabs", async () => {
		await recordConsoleEntry(1, { level: "log", args: ["tab-1"] });
		await recordNetworkEntry(1, {
			requestId: "req-1",
			url: "https://example.com/x",
			method: "GET",
		});
		await recordConsoleEntry(2, { level: "log", args: ["tab-2"] });

		const drained: { tabId: number; kind: string }[] = [];
		await drainTabBuckets(1, async (tabId, kind) => {
			drained.push({ tabId, kind });
			return true;
		});
		expect(drained).toEqual([
			{ tabId: 1, kind: "console" },
			{ tabId: 1, kind: "network" },
		]);

		// Tab 1's buckets are gone; tab 2's survive untouched.
		expect(bucketKeys()).toEqual(["2:console"]);
		const posted: { tabId: number; kind: string }[] = [];
		await flushPending(async (tabId, kind) => {
			posted.push({ tabId, kind });
			return true;
		});
		expect(posted).toEqual([{ tabId: 2, kind: "console" }]);
	});

	it("keeps a closed tab's entries when the drain POST fails", async () => {
		await recordConsoleEntry(1, { level: "log", args: ["tail"] });

		await drainTabBuckets(1, async () => false);

		// Bucket survived, so a later flush can still deliver the tail.
		let retried: number[] = [];
		await flushPending(async (_tabId, _kind, entries) => {
			retried = (entries as { seq: number }[]).map((e) => e.seq);
			return true;
		});
		expect(retried).toEqual([1]);

		// Once delivered, the closed tab's bucket is reaped rather than lingering
		// in session storage for the rest of the browser session.
		expect(bucketKeys()).toEqual([]);
	});

	it("does not discard entries recorded during an in-flight flush", async () => {
		await recordConsoleEntry(1, { level: "log", args: ["first"] });

		// Hold a global flush open, record a tail entry, then close the tab. The
		// drain must post the tail rather than joining the stale flush snapshot.
		let releaseFlush: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			releaseFlush = resolve;
		});
		let signalPosting: () => void = () => {};
		const posting = new Promise<void>((resolve) => {
			signalPosting = resolve;
		});
		const slowFlush = flushPending(async () => {
			signalPosting();
			await gate;
			return true;
		});

		// Only record the tail once the flush has taken its snapshot, so the
		// entry provably post-dates the in-flight pass.
		await posting;
		await recordConsoleEntry(1, { level: "log", args: ["tail"] });
		const drainedSeqs: number[] = [];
		const drain = drainTabBuckets(1, async (_tabId, _kind, entries) => {
			drainedSeqs.push(...(entries as { seq: number }[]).map((e) => e.seq));
			return true;
		});

		releaseFlush();
		await slowFlush;
		await drain;

		expect(drainedSeqs).toEqual([2]);
	});

	it("only flushes entries beyond flushedUpToSeq after partial flush", async () => {
		// Record entries 1, 2 and flush them.
		await recordConsoleEntry(1, { level: "log", args: ["x"] });
		await recordConsoleEntry(1, { level: "log", args: ["y"] });
		await flushPending(async () => true);

		// Record entries 3, 4 -- only these should flush next time.
		await recordConsoleEntry(1, { level: "log", args: ["z"] });
		await recordConsoleEntry(1, { level: "log", args: ["w"] });
		let incrementalFlush: number[] = [];
		await flushPending(async (_tabId, _kind, entries) => {
			incrementalFlush = (entries as { seq: number }[]).map((e) => e.seq);
			return true;
		});
		expect(incrementalFlush).toEqual([3, 4]);
	});
});
