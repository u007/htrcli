/**
 * Refcounted chrome.debugger attach + shared onEvent fan-out. The single
 * attach/detach path for long-lived capture features (network + dialogs) so
 * two features can capture the same tab through one real attach. Follows the
 * project rule that a debugger attach is never held permanently: callers
 * (bounded capture windows) balance every attachShared with a detachShared.
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
	subscribers.clear();
	sharedListenerAttached = false;
}

function impl(): DebuggerImpl {
	if (injected) return injected;
	return chrome.debugger as unknown as DebuggerImpl;
}

const refcounts = new Map<number, number>();
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

// Attach the debugger to a tab, refcounted: the real attach happens only on
// the first holder. Throws verbatim if the real attach fails (DevTools open /
// another client attached) rather than silently continuing.
export async function attachShared(tabId: number): Promise<void> {
	const current = refcounts.get(tabId) ?? 0;
	if (current === 0) {
		ensureSharedListener();
		await impl().attach({ tabId }, "1.3");
	}
	refcounts.set(tabId, current + 1);
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
