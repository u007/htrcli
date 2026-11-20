/**
 * Refcounted chrome.debugger attach + shared onEvent fan-out. The single
 * attach/detach path for every feature that needs the debugger — network and
 * dialog capture, trusted input, upload, and eval — so two features can share
 * one real attach. Follows the project rule that a debugger attach is never
 * held permanently: callers (bounded command or capture windows) balance every
 * attachShared with a detachShared.
 */

type EventCb = (
	source: { tabId?: number },
	method: string,
	params: unknown,
) => void;

interface DebuggerImpl {
	attach: (target: { tabId: number }, version: string) => Promise<void>;
	detach: (target: { tabId: number }) => Promise<void>;
	sendCommand: (
		target: { tabId: number },
		method: string,
		params?: Record<string, unknown>,
	) => Promise<unknown>;
	onEvent: {
		addListener: (cb: EventCb) => void;
		removeListener: (cb: EventCb) => void;
	};
}

let injected: DebuggerImpl | null = null;

// Test seam: swap the real chrome.debugger for a fake. Passing null restores
// the real API and resets internal state so each test starts clean.
export function __setDebuggerImplForTests(impl: DebuggerImpl | null): void {
	injected = impl;
	refcounts.clear();
	attaching.clear();
	subscribers.clear();
	sharedListenerAttached = false;
}

function impl(): DebuggerImpl {
	if (injected) return injected;
	return chrome.debugger as unknown as DebuggerImpl;
}

const refcounts = new Map<number, number>();
const attaching = new Map<number, Promise<void>>();
const subscribers = new Set<EventCb>();
let sharedListenerAttached = false;

function sharedListener(
	source: { tabId?: number },
	method: string,
	params: unknown,
): void {
	for (const cb of subscribers) {
		cb(source, method, params);
	}
}

function ensureSharedListener(): void {
	if (sharedListenerAttached) return;
	impl().onEvent.addListener(sharedListener);
	sharedListenerAttached = true;
}

const STALE_ATTACH = /another debugger is already attached/i;

/**
 * Attach, recovering from an attach this extension itself left behind.
 *
 * A command the caller abandons (client timeout, closed tab) can leave a real
 * attach with no refcount, and every later command then fails with "Another
 * debugger is already attached" until the tab is reloaded. `chrome.debugger`
 * detach only releases *our* session — it cannot release the user's DevTools —
 * so detaching and retrying once is safe: if somebody else holds the tab, the
 * retry fails with the same error, which the caller still gets verbatim.
 */
async function attachWithStaleRecovery(tabId: number): Promise<void> {
	try {
		await impl().attach({ tabId }, "1.3");
		return;
	} catch (err) {
		const message = err instanceof Error ? err.message : String(err);
		if (!STALE_ATTACH.test(message)) {
			throw err;
		}
	}
	try {
		await impl().detach({ tabId });
	} catch (err) {
		// Benign: nothing of ours is held (the tab closed, or the attach was
		// never ours). Logged so it is never a silent swallow.
		console.warn(
			"[HTR NControl] stale debugger detach failed:",
			err instanceof Error ? err.message : err,
		);
	}
	await impl().attach({ tabId }, "1.3");
}

// Attach the debugger to a tab, refcounted: the real attach happens only on
// the first holder. Throws verbatim if the real attach fails (DevTools open /
// another client attached) rather than silently continuing.
//
// Two concurrent callers must not both attempt a real attach: the loser gets
// "Another debugger is already attached", and its caller's compensating
// detachShared would then release the WINNER's hold and kill a live debugger.
// The hold is therefore reserved synchronously and concurrent callers share
// one in-flight promise. A failed attach rolls the reservation back, so a
// caller that detaches in its catch block is a no-op rather than a release of
// somebody else's refcount.
export async function attachShared(tabId: number): Promise<void> {
	const pending = attaching.get(tabId);
	if (pending) {
		await pending;
		refcounts.set(tabId, (refcounts.get(tabId) ?? 0) + 1);
		return;
	}
	const current = refcounts.get(tabId) ?? 0;
	if (current > 0) {
		refcounts.set(tabId, current + 1);
		return;
	}
	// Reserve before awaiting so a concurrent caller sees the hold.
	refcounts.set(tabId, 1);
	const inflight = (async () => {
		try {
			ensureSharedListener();
			await attachWithStaleRecovery(tabId);
		} catch (err) {
			refcounts.delete(tabId);
			// The caller reports the error to the CLI; this is the only trace it
			// leaves in the extension console, so it must not be silent.
			console.warn(
				"[HTR NControl] debugger attach failed:",
				err instanceof Error ? err.message : err,
			);
			throw err;
		}
	})();
	attaching.set(tabId, inflight);
	try {
		await inflight;
	} finally {
		attaching.delete(tabId);
	}
}

// Release one hold on a tab's attach; the real detach happens only when the
// last holder releases.
export async function detachShared(tabId: number): Promise<void> {
	const current = refcounts.get(tabId) ?? 0;
	if (current <= 1) {
		refcounts.delete(tabId);
		if (current === 1) {
			try {
				await impl().detach({ tabId });
			} catch (err) {
				// Benign: the tab may have closed, which auto-detaches the
				// debugger. Log at warn so it is never a silent swallow.
				console.warn("[HTR NControl] debugger detach failed:", err);
			}
		}
		return;
	}
	refcounts.set(tabId, current - 1);
}

// Subscribe to all debugger events. Returns an unsubscribe function.
export function onDebuggerEvent(cb: EventCb): () => void {
	subscribers.add(cb);
	return () => subscribers.delete(cb);
}

// Send a CDP command to an already-attached tab.
export function sendToTab(
	tabId: number,
	method: string,
	params: Record<string, unknown> = {},
): Promise<unknown> {
	return impl().sendCommand({ tabId }, method, params);
}
