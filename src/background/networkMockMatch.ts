/**
 * Pure request-matching for declarative network mocks. Shared by the Chrome
 * (CDP Fetch) and Firefox (webRequest) interceptors so both behave identically.
 */

export interface NetworkMockRule {
	/** Stable id assigned by the CLI (for targeted unmock). */
	id: string;
	/** Glob URL pattern ("*" = any run of chars). */
	urlPattern: string;
	/** Optional HTTP method constraint (case-insensitive). Absent = any. */
	method?: string;
	/** "fulfill" = serve a mock response; "fail" = block the request. */
	kind: "fulfill" | "fail";
	/** Mock response status (fulfill only). Defaults to 200 downstream. */
	status?: number;
	/** Mock response body (fulfill only). */
	body?: string;
	/** Mock response headers (fulfill only). */
	headers?: Record<string, string>;
}

/**
 * Convert a glob to an anchored RegExp. Only "*" is special (→ ".*"); every
 * other character is escaped so URL metachars (. ? / +) match literally.
 */
export function globToRegExp(glob: string): RegExp {
	let out = "^";
	for (const ch of glob) {
		if (ch === "*") {
			out += ".*";
		} else {
			out += ch.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
		}
	}
	out += "$";
	return new RegExp(out);
}

/**
 * Return the first rule whose pattern (and optional method) matches, or null.
 * First-match-wins gives the CLI deterministic precedence by insertion order.
 */
export function matchRule(
	rules: NetworkMockRule[],
	url: string,
	method: string,
): NetworkMockRule | null {
	for (const rule of rules) {
		if (rule.method && rule.method.toUpperCase() !== method.toUpperCase()) {
			continue;
		}
		if (globToRegExp(rule.urlPattern).test(url)) {
			return rule;
		}
	}
	return null;
}
