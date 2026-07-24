/**
 * Chrome network interceptor (declarative mock/block) via CDP Fetch.
 *
 * Rules are registered per tab. On the first rule for a tab we attach
 * chrome.debugger and enable the Fetch domain; each Fetch.requestPaused event
 * is matched against the tab's rules and answered locally (fulfill/fail/
 * continue) — no daemon round-trip, because a paused request cannot wait for a
 * separate short-lived CLI process. On the last rule removed we disable Fetch
 * and detach.
 */

import { matchRule, type NetworkMockRule } from "./networkMockMatch";

interface PausedRequest {
	requestId: string;
	request: { url: string; method: string };
}

export interface FetchDecision {
	method:
		| "Fetch.fulfillRequest"
		| "Fetch.failRequest"
		| "Fetch.continueRequest";
	params: Record<string, unknown>;
}

// Base64 without Buffer (service-worker safe): btoa over a binary string.
function toBase64(s: string): string {
	// Encode UTF-8 first so multibyte bodies survive btoa's latin1 assumption.
	const utf8 = new TextEncoder().encode(s);
	let bin = "";
	for (const byte of utf8) bin += String.fromCharCode(byte);
	return btoa(bin);
}

/**
 * Decide how to answer a paused request. Pure — the caller performs the actual
 * chrome.debugger.sendCommand with the returned method/params.
 */
export function decideRequestPaused(
	rules: NetworkMockRule[],
	paused: PausedRequest,
): FetchDecision {
	const rule = matchRule(rules, paused.request.url, paused.request.method);
	if (!rule) {
		return {
			method: "Fetch.continueRequest",
			params: { requestId: paused.requestId },
		};
	}
	if (rule.kind === "fail") {
		return {
			method: "Fetch.failRequest",
			params: { requestId: paused.requestId, errorReason: "BlockedByClient" },
		};
	}
	const headers = Object.entries(rule.headers ?? {}).map(([name, value]) => ({
		name,
		value,
	}));
	return {
		method: "Fetch.fulfillRequest",
		params: {
			requestId: paused.requestId,
			responseCode: rule.status ?? 200,
			responseHeaders: headers,
			body: toBase64(rule.body ?? ""),
		},
	};
}

// ─── Per-tab rule store + debugger lifecycle ───────────────────────────

const rulesByTab = new Map<number, NetworkMockRule[]>();

export function getRules(tabId: number): NetworkMockRule[] {
	return rulesByTab.get(tabId) ?? [];
}

// Guarded so the module unit-tests (no chrome global) exercise the store and
// decision logic without touching the debugger.
function debuggerAvailable(): boolean {
	return (
		typeof chrome !== "undefined" && typeof chrome.debugger !== "undefined"
	);
}

let listenerRegistered = false;

function ensureEventListener(): void {
	if (listenerRegistered || !debuggerAvailable()) return;
	listenerRegistered = true;
	chrome.debugger.onEvent.addListener((source, method, params) => {
		if (method !== "Fetch.requestPaused" || typeof source.tabId !== "number") {
			return;
		}
		const rules = getRules(source.tabId);
		const decision = decideRequestPaused(
			rules,
			params as unknown as PausedRequest,
		);
		void chrome.debugger.sendCommand(
			{ tabId: source.tabId },
			decision.method,
			decision.params,
		);
	});
}

async function armFetch(tabId: number): Promise<void> {
	if (!debuggerAvailable()) return;
	ensureEventListener();
	const target = { tabId };
	// Attach is idempotent-ish: a second attach throws "Already attached", which
	// we tolerate so re-arming an already-armed tab is safe.
	try {
		await chrome.debugger.attach(target, "1.3");
	} catch (err) {
		if (!String(err).includes("Already attached")) {
			throw err;
		}
	}
	await chrome.debugger.sendCommand(target, "Fetch.enable", {});
}

async function disarmFetch(tabId: number): Promise<void> {
	if (!debuggerAvailable()) return;
	const target = { tabId };
	try {
		await chrome.debugger.sendCommand(target, "Fetch.disable", {});
		await chrome.debugger.detach(target);
	} catch (err) {
		console.warn("[HTR NControl] disarmFetch:", err);
	}
}

/** Merge rules for a tab; arm Fetch on the first rule. */
export async function addRules(
	tabId: number,
	rules: NetworkMockRule[],
): Promise<void> {
	const existing = rulesByTab.get(tabId) ?? [];
	const wasEmpty = existing.length === 0;
	rulesByTab.set(tabId, [...existing, ...rules]);
	if (wasEmpty && (rulesByTab.get(tabId)?.length ?? 0) > 0) {
		await armFetch(tabId);
	}
}

/** Remove rules for a tab; disarm Fetch when none remain. */
export async function removeRules(
	tabId: number,
	opts: { all?: boolean; urlPattern?: string },
): Promise<void> {
	if (opts.all) {
		rulesByTab.delete(tabId);
		await disarmFetch(tabId);
		return;
	}
	const kept = (rulesByTab.get(tabId) ?? []).filter(
		(r) => r.urlPattern !== opts.urlPattern,
	);
	if (kept.length === 0) {
		rulesByTab.delete(tabId);
		await disarmFetch(tabId);
	} else {
		rulesByTab.set(tabId, kept);
	}
}
