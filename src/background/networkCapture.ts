/**
 * In-memory assembly of CDP Network.* events into completed NetworkEntry
 * records. Pure and dependency-free so it can be unit-tested without a real
 * debugger. Task 4 wires this to chrome.debugger via debuggerManager.
 */

import type { NetworkEntry } from "../types/recording";

interface Inflight {
	requestId: string;
	url: string;
	method: string;
	requestHeaders?: Record<string, string>;
	startMs: number; // CDP monotonic timestamp in ms
	status?: number;
	responseHeaders?: Record<string, string>;
}

interface RequestWillBeSentParams {
	requestId: string;
	request: { url: string; method: string; headers?: Record<string, string> };
	timestamp: number; // CDP Network.MonotonicTime (seconds)
}

interface ResponseReceivedParams {
	requestId: string;
	response: { status: number; headers?: Record<string, string> };
}

interface LoadingDoneParams {
	requestId: string;
	timestamp: number; // seconds
}

export class NetworkCaptureBuffer {
	private inflight = new Map<string, Inflight>();

	onRequestWillBeSent(params: RequestWillBeSentParams): void {
		this.inflight.set(params.requestId, {
			requestId: params.requestId,
			url: params.request.url,
			method: params.request.method,
			requestHeaders: params.request.headers,
			startMs: params.timestamp * 1000,
		});
	}

	onResponseReceived(params: ResponseReceivedParams): void {
		const r = this.inflight.get(params.requestId);
		if (!r) return;
		r.status = params.response.status;
		r.responseHeaders = params.response.headers;
	}

	onLoadingFinished(params: LoadingDoneParams): NetworkEntry | null {
		return this.complete(params);
	}

	onLoadingFailed(params: LoadingDoneParams): NetworkEntry | null {
		return this.complete(params);
	}

	private complete(params: LoadingDoneParams): NetworkEntry | null {
		const r = this.inflight.get(params.requestId);
		if (!r) return null;
		this.inflight.delete(params.requestId);
		const durationMs = Math.max(
			0,
			Math.round(params.timestamp * 1000 - r.startMs),
		);
		return {
			requestId: r.requestId,
			url: r.url,
			method: r.method,
			status: r.status,
			requestHeaders: r.requestHeaders,
			responseHeaders: r.responseHeaders,
			durationMs,
		};
	}
}
