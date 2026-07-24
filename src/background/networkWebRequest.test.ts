import { describe, expect, it } from "bun:test";
import { mapWebRequestEntry } from "./networkWebRequest";

describe("mapWebRequestEntry", () => {
	it("maps a completed webRequest detail into a NetworkEntry", () => {
		const entry = mapWebRequestEntry({
			requestId: "42",
			url: "https://example.com/api/ping",
			method: "POST",
			statusCode: 204,
			timeStamp: 1_000,
			startedMs: 950,
		});
		expect(entry).toEqual({
			requestId: "42",
			url: "https://example.com/api/ping",
			method: "POST",
			status: 204,
			durationMs: 50,
		});
	});

	it("omits status and duration when unknown", () => {
		const entry = mapWebRequestEntry({
			requestId: "43",
			url: "https://example.com/x",
			method: "GET",
			timeStamp: 100,
		});
		expect(entry.status).toBeUndefined();
		expect(entry.durationMs).toBeUndefined();
	});
});
