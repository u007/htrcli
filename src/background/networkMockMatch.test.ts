import { describe, expect, it } from "bun:test";
import {
	globToRegExp,
	matchRule,
	type NetworkMockRule,
} from "./networkMockMatch";

describe("globToRegExp", () => {
	it("treats * as any run of chars and escapes regex metachars", () => {
		const re = globToRegExp("https://api.example.com/*/users");
		expect(re.test("https://api.example.com/v1/users")).toBe(true);
		expect(re.test("https://api.example.com/v1/orders")).toBe(false);
	});

	it("anchors fully (no partial match)", () => {
		const re = globToRegExp("https://x.com/a");
		expect(re.test("https://x.com/a/b")).toBe(false);
	});
});

describe("matchRule", () => {
	const rules: NetworkMockRule[] = [
		{ id: "1", urlPattern: "https://api.example.com/*", kind: "fail" },
		{
			id: "2",
			urlPattern: "https://api.example.com/users",
			method: "POST",
			kind: "fulfill",
			status: 201,
			body: "{}",
		},
	];

	it("returns the first matching rule in order", () => {
		const m = matchRule(rules, "https://api.example.com/users", "GET");
		expect(m?.id).toBe("1");
	});

	it("respects a method constraint", () => {
		// The GET does not match rule 2 (POST-only); it falls to rule 1.
		expect(matchRule(rules, "https://api.example.com/users", "POST")?.id).toBe(
			"1",
		);
		// A method-less rule matches any method.
		expect(
			matchRule([rules[0]], "https://api.example.com/z", "DELETE")?.id,
		).toBe("1");
	});

	it("returns null when nothing matches", () => {
		expect(matchRule(rules, "https://other.com/", "GET")).toBeNull();
	});
});
