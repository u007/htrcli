import type {
	ConsoleEntry,
	DialogEntry,
	NetworkEntry,
} from "../types/recording";

export type ConsoleEntryData = ConsoleEntry;
export type NetworkEntryData = NetworkEntry;
export type DialogEntryData = DialogEntry;
export type BufferedEventData =
	| ConsoleEntryData
	| NetworkEntryData
	| DialogEntryData;

export type EventKind = "console" | "network" | "dialog";

export interface BufferedEvent {
	seq: number;
	kind: EventKind;
	timestamp: number;
	data: BufferedEventData;
}

interface BufferedBucket {
	nextSeq: number;
	entries: BufferedEvent[];
	/** Highest seq that has been successfully flushed to the daemon.
	 *  On daemon restart (generation change) this is reset to 0 so all
	 *  buffered entries are replayed through the ingest endpoint. */
	flushedUpToSeq: number;
	/** Set when the owning tab closed while entries were still undelivered.
	 *  The bucket is reaped by the next flush that lands its tail, so a
	 *  closed tab can't leak buckets for the rest of the browser session. */
	tabClosed?: true;
}

interface BufferedState {
	buckets: Record<string, BufferedBucket>;
	generation: number | null;
}

export type FlushPoster = (
	tabId: number,
	kind: string,
	entries: BufferedEvent[],
) => Promise<boolean>;

const STORAGE_KEY = "htrncontrol:event-store";
const MAX_BUFFERED_EVENTS = 500;

let state: BufferedState | null = null;
let stateLoadPromise: Promise<BufferedState> | null = null;
let flushInFlight: Promise<void> | null = null;
// Serializes every pass over the buckets. The daemon's Ingest re-numbers and
// appends blindly, so two overlapping passes would post the same entries twice
// and duplicate them in the daemon's store.
let opChain: Promise<void> = Promise.resolve();
// At most one storage write is active at a time. Concurrent recorders share
// the in-flight write and start one follow-up write only when their mutation
// happened after that write's snapshot.
let stateVersion = 0;
let persistedStateVersion = 0;
let saveInFlight: Promise<void> | null = null;

function serializeBucketPass(work: () => Promise<void>): Promise<void> {
	const run = opChain.then(work, work);
	opChain = run.catch(() => {});
	return run;
}

// Test seam: reset the cached session snapshot so each test starts clean.
export function __resetEventStoreForTests(): void {
	state = null;
	stateLoadPromise = null;
	flushInFlight = null;
	opChain = Promise.resolve();
	stateVersion = 0;
	persistedStateVersion = 0;
	saveInFlight = null;
}

function bucketKey(tabId: number, kind: EventKind): string {
	return `${tabId}:${kind}`;
}

function emptyState(): BufferedState {
	return { buckets: {}, generation: null };
}

async function loadState(): Promise<BufferedState> {
	if (state) return state;
	if (!stateLoadPromise) {
		stateLoadPromise = chrome.storage.session
			.get(STORAGE_KEY)
			.then((result) => {
				const stored = result[STORAGE_KEY] as BufferedState | undefined;
				state = stored ?? emptyState();
				return state;
			});
	}
	return stateLoadPromise;
}

async function saveState(): Promise<void> {
	while (state && persistedStateVersion < stateVersion) {
		if (!saveInFlight) {
			const version = stateVersion;
			const snapshot = structuredClone(state);
			saveInFlight = chrome.storage.session
				.set({ [STORAGE_KEY]: snapshot })
				.then(() => {
					persistedStateVersion = Math.max(persistedStateVersion, version);
				})
				.finally(() => {
					saveInFlight = null;
				});
		}
		const save = saveInFlight;
		if (!save) continue;
		await save;
	}
}

function markStateDirty(): void {
	stateVersion += 1;
}

function getOrCreateBucket(
	currentState: BufferedState,
	tabId: number,
	kind: EventKind,
): BufferedBucket {
	const key = bucketKey(tabId, kind);
	let bucket = currentState.buckets[key];
	if (!bucket) {
		bucket = { nextSeq: 1, entries: [], flushedUpToSeq: 0 };
		currentState.buckets[key] = bucket;
	}
	return bucket;
}

function trimBucket(bucket: BufferedBucket): void {
	if (bucket.entries.length <= MAX_BUFFERED_EVENTS) return;
	bucket.entries.splice(0, bucket.entries.length - MAX_BUFFERED_EVENTS);
}

function normalizeLevel(
	level: ConsoleEntryData["level"],
): ConsoleEntryData["level"] {
	return level;
}

// Persist the last daemon generation the native host greeted us with.
export async function setLastKnownGeneration(
	generation: number,
): Promise<void> {
	const currentState = await loadState();
	currentState.generation = generation;
	markStateDirty();
	await saveState();
}

// Read the last daemon generation the native host greeted us with.
export async function getGeneration(): Promise<number | null> {
	const currentState = await loadState();
	return currentState.generation;
}

// Prepare for a daemon restart (generation change). Reset flushedUpToSeq to 0
// for every bucket so the next flushPending call replays ALL buffered entries
// through the ingest endpoint, repopulating the daemon's fresh in-memory store.
export async function resetForResync(): Promise<void> {
	const currentState = await loadState();
	for (const key of Object.keys(currentState.buckets)) {
		currentState.buckets[key].flushedUpToSeq = 0;
	}
	markStateDirty();
	await saveState();
}

// Record one captured event of any kind in durable session storage.
export async function recordEvent(
	tabId: number,
	kind: EventKind,
	data: BufferedEventData,
): Promise<void> {
	if (tabId <= 0) return;
	const currentState = await loadState();
	const bucket = getOrCreateBucket(currentState, tabId, kind);
	bucket.entries.push({
		seq: bucket.nextSeq,
		kind,
		timestamp: Date.now(),
		data,
	});
	bucket.nextSeq += 1;
	trimBucket(bucket);
	markStateDirty();
	await saveState();
}

// Flush a closing tab's buckets and discard the ones fully delivered. Buckets
// are keyed by tab ID and a closed tab's ID is never reused, so without the
// discard they accumulate in session storage for the lifetime of the browser
// session and every later save re-serializes them.
export async function drainTabBuckets(
	tabId: number,
	postEventsToDaemon: FlushPoster,
): Promise<void> {
	// Own pass rather than delegating to flushPending: that call coalesces onto
	// an in-flight flush whose snapshot predates the tab's tail entries, which
	// the discard below would then delete unsent.
	await serializeBucketPass(async () => {
		const currentState = await loadState();
		const prefix = `${tabId}:`;
		const keys = Object.keys(currentState.buckets).filter((key) =>
			key.startsWith(prefix),
		);
		if (keys.length === 0) return;

		await flushBuckets(keys, postEventsToDaemon);

		let dirty = false;
		for (const key of keys) {
			const bucket = currentState.buckets[key];
			if (!bucket) continue;
			const lastSeq = bucket.entries[bucket.entries.length - 1]?.seq ?? 0;
			if (bucket.flushedUpToSeq < lastSeq) {
				// Tail never reached the daemon (POST returned false, or the daemon
				// is down). Keep it for a later flush and mark the bucket so that
				// flush reaps it once delivered.
				bucket.tabClosed = true;
			} else {
				delete currentState.buckets[key];
			}
			dirty = true;
		}
		if (dirty) {
			markStateDirty();
			await saveState();
		}
	});
}

// Record a console entry in durable session storage.
export async function recordConsoleEntry(
	tabId: number,
	entry: ConsoleEntryData,
): Promise<void> {
	await recordEvent(tabId, "console", {
		level: normalizeLevel(entry.level),
		args: [...entry.args],
		source: entry.source,
	});
}

// Record a network entry in durable session storage.
export async function recordNetworkEntry(
	tabId: number,
	entry: NetworkEntryData,
): Promise<void> {
	await recordEvent(tabId, "network", entry);
}

// Record a dialog entry in durable session storage.
export async function recordDialogEntry(
	tabId: number,
	entry: DialogEntryData,
): Promise<void> {
	await recordEvent(tabId, "dialog", entry);
}

// Post the unflushed tail of each named bucket and advance its watermark.
async function flushBuckets(
	bucketKeys: string[],
	postEventsToDaemon: FlushPoster,
): Promise<void> {
	const currentState = await loadState();
	for (const key of bucketKeys) {
		const bucket = currentState.buckets[key];
		if (!bucket || bucket.entries.length === 0) continue;

		const [tabIDText, kind] = key.split(":", 2);
		const tabID = Number.parseInt(tabIDText, 10);
		if (!Number.isFinite(tabID) || tabID <= 0) continue;

		// Only send entries not yet flushed (seq > flushedUpToSeq).
		const flushedUpTo = bucket.flushedUpToSeq ?? 0;
		const unflushed = bucket.entries.filter((entry) => entry.seq > flushedUpTo);
		if (unflushed.length === 0) continue;

		const snapshot = unflushed.map((entry) => ({ ...entry }));
		const sent = await postEventsToDaemon(tabID, kind, snapshot);
		if (!sent) continue;

		// Advance the watermark instead of deleting entries. Entries remain in
		// the buffer so they can be replayed if the daemon restarts (generation
		// change resets flushedUpToSeq back to 0). The 500-entry cap and
		// eviction (oldest first) still apply, so flushed entries never
		// accumulate forever.
		const lastSeq = snapshot[snapshot.length - 1]?.seq ?? 0;
		bucket.flushedUpToSeq = Math.max(flushedUpTo, lastSeq);
		markStateDirty();

		// The owning tab is gone and everything buffered is now delivered, so the
		// bucket has no further use — drop it instead of re-serializing it on
		// every subsequent save.
		const tailSeq = bucket.entries[bucket.entries.length - 1]?.seq ?? 0;
		if (bucket.tabClosed && bucket.flushedUpToSeq >= tailSeq) {
			delete currentState.buckets[key];
		}

		// Persist after each successful bucket POST so a service-worker death
		// mid-flush doesn't replay already-sent entries on restart.
		await saveState();
	}
}

// Flush every buffered bucket through the caller-provided POST function.
// Returns once the current snapshot has been attempted. Entries remain buffered
// if the callback returns false or throws.
export async function flushPending(
	postEventsToDaemon: FlushPoster,
): Promise<void> {
	if (flushInFlight) {
		return flushInFlight;
	}

	const run = serializeBucketPass(async () => {
		const currentState = await loadState();
		await flushBuckets(Object.keys(currentState.buckets), postEventsToDaemon);
	});

	flushInFlight = run.finally(() => {
		flushInFlight = null;
	});

	await flushInFlight;
}
