/**
 * Firefox network capture via browser.webRequest (exposed as chrome.webRequest
 * through the polyfill). Always-on, metadata-only — webRequest cannot cheaply
 * read response bodies, so NetworkEntry.body is omitted on Firefox. This is the
 * documented Chrome/Firefox asymmetry (see the plan's Global Constraints).
 */

import type { NetworkEntry } from "../types/recording";
import { recordNetworkEntry } from "./eventStore";

interface WebRequestCompleted {
	requestId: string;
	url: string;
	method: string;
	statusCode?: number;
	timeStamp: number; // ms epoch when completed
	startedMs?: number; // ms epoch when the request began (tracked locally)
}

export function mapWebRequestEntry(details: WebRequestCompleted): NetworkEntry {
	const entry: NetworkEntry = {
		requestId: details.requestId,
		url: details.url,
		method: details.method,
	};
	if (typeof details.statusCode === "number") entry.status = details.statusCode;
	if (typeof details.startedMs === "number") {
		entry.durationMs = Math.max(
			0,
			Math.round(details.timeStamp - details.startedMs),
		);
	}
	return entry;
}

// Register always-on webRequest observers. tabId comes from webRequest details
// (details.tabId); requests with tabId < 0 (e.g. background/service requests)
// are ignored since the event buffer is per real tab.
export function startWebRequestCapture(): void {
	if (typeof chrome === "undefined" || !chrome.webRequest) return;

	const startTimes = new Map<string, number>();
	// Requests that never reach onCompleted/onErrorOccurred (tab torn down
	// mid-flight, some aborts) would otherwise sit here for the lifetime of the
	// background script. Evict oldest-first; Map preserves insertion order.
	const MAX_TRACKED_REQUESTS = 1000;

	chrome.webRequest.onBeforeRequest.addListener(
		(details: { requestId: string; timeStamp: number }) => {
			startTimes.set(details.requestId, details.timeStamp);
			while (startTimes.size > MAX_TRACKED_REQUESTS) {
				const oldest = startTimes.keys().next();
				if (oldest.done) break;
				startTimes.delete(oldest.value);
			}
		},
		{ urls: ["http://*/*", "https://*/*"] },
	);

	const onDone = (details: {
		requestId: string;
		url: string;
		method: string;
		statusCode?: number;
		timeStamp: number;
		tabId: number;
	}): void => {
		const startedMs = startTimes.get(details.requestId);
		startTimes.delete(details.requestId);
		if (details.tabId < 0) return;
		const entry = mapWebRequestEntry({ ...details, startedMs });
		void recordNetworkEntry(details.tabId, entry);
	};

	chrome.webRequest.onCompleted.addListener(onDone, {
		urls: ["http://*/*", "https://*/*"],
	});
	chrome.webRequest.onErrorOccurred.addListener(onDone, {
		urls: ["http://*/*", "https://*/*"],
	});
}
