import { describe, expect, it } from "bun:test";
import { NetworkCaptureBuffer } from "./networkCapture";

describe("NetworkCaptureBuffer", () => {
	it("assembles a completed entry from request → response → finished", () => {
		const buf = new NetworkCaptureBuffer();
		buf.onRequestWillBeSent({
			requestId: "1",
			request: {
				url: "https://example.com/api",
				method: "GET",
				headers: { accept: "application/json" },
			},
			timestamp: 1000, // CDP monotonic seconds
		});
		buf.onResponseReceived({
			requestId: "1",
			response: {
				status: 200,
				headers: { "content-type": "application/json" },
			},
		});
		const entry = buf.onLoadingFinished({ requestId: "1", timestamp: 1000.5 });
		expect(entry).not.toBeNull();
		expect(entry?.url).toBe("https://example.com/api");
		expect(entry?.method).toBe("GET");
		expect(entry?.status).toBe(200);
		expect(entry?.durationMs).toBe(500);
		expect(entry?.requestHeaders?.accept).toBe("application/json");
		expect(entry?.responseHeaders?.["content-type"]).toBe("application/json");
	});

	it("returns null for a finished event with no matching request", () => {
		const buf = new NetworkCaptureBuffer();
		expect(
			buf.onLoadingFinished({ requestId: "missing", timestamp: 5 }),
		).toBeNull();
	});

	it("assembles a failed request with no status", () => {
		const buf = new NetworkCaptureBuffer();
		buf.onRequestWillBeSent({
			requestId: "2",
			request: { url: "https://example.com/down", method: "POST" },
			timestamp: 2,
		});
		const entry = buf.onLoadingFailed({ requestId: "2", timestamp: 2.1 });
		expect(entry?.url).toBe("https://example.com/down");
		expect(entry?.status).toBeUndefined();
		expect(entry?.durationMs).toBe(100);
	});
});
