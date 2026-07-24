import { describe, expect, it } from "bun:test";
import { decideBlockingResponse } from "./networkMockFirefox";
import type { NetworkMockRule } from "./networkMockMatch";

const rules: NetworkMockRule[] = [
	{ id: "1", urlPattern: "https://ads.example.com/*", kind: "fail" },
	{
		id: "2",
		urlPattern: "https://api.example.com/*",
		kind: "fulfill",
		status: 200,
		body: '{"ok":true}',
		headers: { "Content-Type": "application/json" },
	},
];

describe("decideBlockingResponse (Firefox)", () => {
	it("cancels a blocked request", () => {
		expect(
			decideBlockingResponse(rules, "https://ads.example.com/x.js", "GET"),
		).toEqual({
			cancel: true,
		});
	});

	it("redirects a mocked request to a data: URL carrying the body", () => {
		const r = decideBlockingResponse(
			rules,
			"https://api.example.com/users",
			"GET",
		);
		expect(r.redirectUrl).toBeDefined();
		expect(r.redirectUrl).toContain("data:application/json");
		// decode the data: URL payload
		const encoded = (r.redirectUrl as string).split(",")[1];
		expect(decodeURIComponent(encoded)).toBe('{"ok":true}');
	});

	it("returns an empty object (no interception) when nothing matches", () => {
		expect(decideBlockingResponse(rules, "https://other.com/", "GET")).toEqual(
			{},
		);
	});
});
