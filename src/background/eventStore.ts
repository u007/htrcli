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

// Test seam: reset the cached session snapshot so each test starts clean.
export function __resetEventStoreForTests(): void {
	state = null;
	stateLoadPromise = null;
	flushInFlight = null;
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
	if (!state) return;
	await chrome.storage.session.set({ [STORAGE_KEY]: state });
}

function getOrCreateBucket(
	currentState: BufferedState,
	tabId: number,
	kind: EventKind,
): BufferedBucket {
	const key = bucketKey(tabId, kind);
	let bucket = currentState.buckets[key];
	if (!bucket) {
		bucket = { nextSeq: 1, entries: [] };
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
	await saveState();
}

// Read the last daemon generation the native host greeted us with.
export async function getGeneration(): Promise<number | null> {
	const currentState = await loadState();
	return currentState.generation;
}

// Clear any resync bookkeeping. The current implementation keeps only pending
// entries, so a daemon restart just means the next flush will resend whatever
// is still buffered.
export async function resetForResync(): Promise<void> {
	await loadState();
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
	await saveState();
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

async function flushPendingOnce(
	postEventsToDaemon: FlushPoster,
): Promise<void> {
	const currentState = await loadState();
	const bucketKeys = Object.keys(currentState.buckets);
	for (const key of bucketKeys) {
		const bucket = currentState.buckets[key];
		if (!bucket || bucket.entries.length === 0) continue;

		const [tabIDText, kind] = key.split(":", 2);
		const tabID = Number.parseInt(tabIDText, 10);
		if (!Number.isFinite(tabID) || tabID <= 0) continue;

		const snapshot = bucket.entries.map((entry) => ({ ...entry }));
		const sent = await postEventsToDaemon(tabID, kind, snapshot);
		if (!sent) continue;

		const lastSeq = snapshot[snapshot.length - 1]?.seq ?? 0;
		bucket.entries = bucket.entries.filter((entry) => entry.seq > lastSeq);

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

	flushInFlight = flushPendingOnce(postEventsToDaemon).finally(() => {
		flushInFlight = null;
	});

	return flushInFlight;
}
