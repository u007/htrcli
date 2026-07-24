import { beforeEach, describe, expect, it } from "bun:test";
import {
	addRules,
	decideRequestPaused,
	getRules,
	removeRules,
} from "./networkMock";
import type { NetworkMockRule } from "./networkMockMatch";

const fulfillRule: NetworkMockRule = {
	id: "1",
	urlPattern: "https://api.example.com/*",
	kind: "fulfill",
	status: 201,
	body: '{"ok":true}',
	headers: { "Content-Type": "application/json" },
};
const blockRule: NetworkMockRule = {
	id: "2",
	urlPattern: "https://ads.example.com/*",
	kind: "fail",
};

describe("decideRequestPaused", () => {
	it("fulfills a matching mock rule with base64 body and status", () => {
		const d = decideRequestPaused([fulfillRule], {
			requestId: "R1",
			request: { url: "https://api.example.com/users", method: "GET" },
		});
		expect(d.method).toBe("Fetch.fulfillRequest");
		expect(d.params.requestId).toBe("R1");
		expect(d.params.responseCode).toBe(201);
		// body is base64-encoded per CDP Fetch.fulfillRequest contract.
		expect(atob(d.params.body as string)).toBe('{"ok":true}');
	});

	it("fails a matching block rule", () => {
		const d = decideRequestPaused([blockRule], {
			requestId: "R2",
			request: { url: "https://ads.example.com/x.js", method: "GET" },
		});
		expect(d.method).toBe("Fetch.failRequest");
		expect(d.params.errorReason).toBe("BlockedByClient");
	});

	it("continues a non-matching request untouched", () => {
		const d = decideRequestPaused([fulfillRule], {
			requestId: "R3",
			request: { url: "https://other.com/", method: "GET" },
		});
		expect(d.method).toBe("Fetch.continueRequest");
		expect(d.params.requestId).toBe("R3");
	});
});

describe("rule store", () => {
	beforeEach(async () => {
		await removeRules(1, { all: true });
	});

	it("merges added rules and removes by pattern / all", async () => {
		await addRules(1, [fulfillRule, blockRule]);
		expect(getRules(1).map((r) => r.id)).toEqual(["1", "2"]);
		await removeRules(1, { urlPattern: "https://ads.example.com/*" });
		expect(getRules(1).map((r) => r.id)).toEqual(["1"]);
		await removeRules(1, { all: true });
		expect(getRules(1)).toEqual([]);
	});
});
