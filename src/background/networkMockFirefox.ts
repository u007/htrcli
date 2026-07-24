/**
 * Firefox network interceptor (declarative mock/block) via webRequest.
 *
 * Firefox lacks chrome.debugger, so we use a blocking webRequest listener.
 * Limitations vs. the Chrome CDP path (documented, not silently swallowed):
 *  - a mocked body is served by redirecting to a data: URL, so the final URL
 *    differs from the requested one and only the Content-Type is honored
 *    (the status code cannot be set via a redirect);
 *  - block is exact (cancel:true).
 */

import { matchRule, type NetworkMockRule } from "./networkMockMatch";

/**
 * Pure decision for the blocking listener. Returns a webRequest
 * BlockingResponse: {cancel:true} to block, {redirectUrl} to mock, or {} to
 * leave the request untouched.
 */
export function decideBlockingResponse(
	rules: NetworkMockRule[],
	url: string,
	method: string,
): { cancel?: boolean; redirectUrl?: string } {
	const rule = matchRule(rules, url, method);
	if (!rule) return {};
	if (rule.kind === "fail") return { cancel: true };
	const contentType = rule.headers?.["Content-Type"] ?? "text/plain";
	const redirectUrl = `data:${contentType},${encodeURIComponent(rule.body ?? "")}`;
	return { redirectUrl };
}

const firefoxRulesByTab = new Map<number, NetworkMockRule[]>();
let firefoxListener:
	| ((details: { url: string; method: string; tabId: number }) => {
			cancel?: boolean;
			redirectUrl?: string;
	  })
	| null = null;

function webRequestAvailable(): boolean {
	return (
		typeof browser !== "undefined" &&
		typeof (browser as { webRequest?: unknown }).webRequest !== "undefined"
	);
}

function ensureFirefoxListener(): void {
	if (firefoxListener || !webRequestAvailable()) return;
	firefoxListener = (details) => {
		const rules = firefoxRulesByTab.get(details.tabId) ?? [];
		return decideBlockingResponse(rules, details.url, details.method);
	};
	// `browser` is Firefox's WebExtension namespace (via webextension-polyfill).
	(
		browser as unknown as {
			webRequest: {
				onBeforeRequest: {
					addListener: (
						cb: typeof firefoxListener,
						filter: { urls: string[] },
						extra: string[],
					) => void;
				};
			};
		}
	).webRequest.onBeforeRequest.addListener(
		firefoxListener,
		{ urls: ["<all_urls>"] },
		["blocking"],
	);
}

export function addRulesFirefox(tabId: number, rules: NetworkMockRule[]): void {
	ensureFirefoxListener();
	const existing = firefoxRulesByTab.get(tabId) ?? [];
	firefoxRulesByTab.set(tabId, [...existing, ...rules]);
}

export function removeRulesFirefox(
	tabId: number,
	opts: { all?: boolean; urlPattern?: string },
): void {
	if (opts.all) {
		firefoxRulesByTab.delete(tabId);
		return;
	}
	const kept = (firefoxRulesByTab.get(tabId) ?? []).filter(
		(r) => r.urlPattern !== opts.urlPattern,
	);
	if (kept.length === 0) firefoxRulesByTab.delete(tabId);
	else firefoxRulesByTab.set(tabId, kept);
}
