import { describe, expect, it } from "bun:test";
import {
	getSourceFromStack,
	type OwnScript,
	ownScriptFromStack,
} from "./stackSource";

const OWN: OwnScript = {
	url: "chrome-extension://ID/assets/chunk-CaEYyn41.js",
	path: "assets/chunk-CaEYyn41.js",
};

// Shapes observed in the wild (Notion bug report, 2026-10-08):
//  - V8 default: "Error" header, then `at fn (url:line:col)` frames.
//  - Page-installed Error.prepareStackTrace: arbitrary strings, possibly
//    headerless, possibly scheme-stripped (`assets/chunk-X.js:2 (fn)`),
//    possibly a non-string (array of CallSites) — `.split` would throw.
//  - SpiderMonkey (Firefox): `fn@url:line:col`, header presence varies.
// The wrapper frame must NEVER be reported as the console entry's source.

describe("getSourceFromStack", () => {
	it("returns the first page frame from a default V8 stack (own chunk frame skipped)", () => {
		const stack = [
			"Error",
			"    at console.warn (chrome-extension://ID/assets/chunk-CaEYyn41.js:2:1234)",
			"    at e.warn (https://app.notion.com/_assets/transport-support.js:1:5)",
			"    at Object.<anonymous> (https://app.notion.com/_assets/56643.js:1:9)",
		].join("\n");
		expect(getSourceFromStack(stack)).toBe(
			"e.warn (https://app.notion.com/_assets/transport-support.js:1:5)",
		);
	});

	it("returns the FIRST page frame from a headerless custom prepareStackTrace string", () => {
		// Own frame at index 0, page frames at 1 and 2. The old slice(2)
		// started at index 2 and reported the second page frame.
		const stack = [
			"assets/chunk-CaEYyn41.js:2 (console.<computed>)",
			"https://app.notion.com/_assets/transport-support.js:1 (e.warn)",
			"https://app.notion.com/_assets/56643.js:1 (o)",
		].join("\n");
		expect(getSourceFromStack(stack, OWN)).toBe(
			"https://app.notion.com/_assets/transport-support.js:1 (e.warn)",
		);
	});

	it("skips Firefox-style frames (fn@url:line:col) belonging to the extension", () => {
		const stack = [
			"warn@moz-extension://UUID/contentScript/consoleCapture.js:88:20",
			"e@https://app.notion.com/_assets/transport-support.js:1:5",
		].join("\n");
		expect(getSourceFromStack(stack)).toBe(
			"e@https://app.notion.com/_assets/transport-support.js:1:5",
		);
	});

	it("returns undefined when every frame belongs to the extension", () => {
		const stack = [
			"Error",
			"    at console.warn (chrome-extension://ID/assets/chunk-CaEYyn41.js:2:1234)",
			"    at inner (chrome-extension://ID/assets/chunk-CaEYyn41.js:3:9)",
		].join("\n");
		expect(getSourceFromStack(stack)).toBeUndefined();
	});

	it("returns undefined when the wrapper frame has no scheme and no ownPath is known", () => {
		// Headerless, scheme-stripped, ownPath undefined (e.g. a page
		// prepareStackTrace mangled the module-load stack): reporting the
		// wrapper would be worse than reporting nothing.
		const stack = [
			"assets/chunk-CaEYyn41.js:2 (console.<computed>)",
			"https://app.notion.com/_assets/transport-support.js:1 (e.warn)",
		].join("\n");
		expect(getSourceFromStack(stack)).toBeUndefined();
	});

	it("returns undefined for a non-string stack instead of throwing", () => {
		// A page's Error.prepareStackTrace may return CallSite[] (or anything);
		// the old implementation called stack.split() and threw INSIDE the
		// page's console call.
		const callSites = [{ getFileName: () => "https://x.test/a.js" }];
		expect(getSourceFromStack(callSites)).toBeUndefined();
		expect(getSourceFromStack(undefined)).toBeUndefined();
		expect(getSourceFromStack("")).toBeUndefined();
	});

	it("skips header/prose/line-number/code lines that are not frames", () => {
		const stack = [
			"Stack Trace",
			"1",
			"2",
			'error(x){return y.replace(/^at\\s+/,"")}',
			"    at pageCall (https://app.test/app.js:7:3)",
		].join("\n");
		expect(getSourceFromStack(stack)).toBe(
			"pageCall (https://app.test/app.js:7:3)",
		);
	});

	it("skips locationless frames like (native) and <anonymous>", () => {
		const stack = [
			"Error",
			"    at console.warn (chrome-extension://ID/assets/chunk-CaEYyn41.js:2:1234)",
			"    at Object.foo (native)",
			"    at pageCall (https://app.test/app.js:7:3)",
		].join("\n");
		expect(getSourceFromStack(stack)).toBe(
			"pageCall (https://app.test/app.js:7:3)",
		);
	});
});

describe("getSourceFromStack: frames without a scheme", () => {
	it("reports an <anonymous> frame (DevTools input, new Function) as page code", () => {
		const stack = [
			"Error",
			"    at console.log (chrome-extension://ID/assets/chunk-CaEYyn41.js:2:1234)",
			"    at <anonymous>:1:5",
		].join("\n");
		expect(getSourceFromStack(stack)).toBe("<anonymous>:1:5");
	});

	it("reports a scheme-stripped page frame when the own path is known", () => {
		// A page prepareStackTrace stripped every scheme; the own path was
		// captured at load, so a frame that fails it is positively page code.
		const stack = [
			"assets/chunk-CaEYyn41.js:2 (console.<computed>)",
			"vendor/polyfill.js:10 (shim)",
			"https://app.test/app.js:7 (pageCall)",
		].join("\n");
		expect(getSourceFromStack(stack, OWN)).toBe("vendor/polyfill.js:10 (shim)");
	});

	it("does not treat a page frame at a generic own path as the wrapper", () => {
		// The own script is `index.js` on the extension origin. A page script at
		// `https://site.test/index.js` must still be reported: the old substring
		// check dropped it.
		const own: OwnScript = {
			url: "chrome-extension://ID/index.js",
			path: "index.js",
		};
		const stack = [
			"Error",
			"    at console.warn (chrome-extension://ID/index.js:2:1)",
			"    at pageCall (https://site.test/index.js:7:3)",
		].join("\n");
		expect(getSourceFromStack(stack, own)).toBe(
			"pageCall (https://site.test/index.js:7:3)",
		);
	});

	it("recognises own frames stripped to host+path or to a leading-slash path", () => {
		const stack = [
			"ID/assets/chunk-CaEYyn41.js:2 (wrapper)",
			"/assets/chunk-CaEYyn41.js:3 (wrapper)",
			"https://app.test/app.js:7 (pageCall)",
		].join("\n");
		expect(getSourceFromStack(stack, OWN)).toBe(
			"https://app.test/app.js:7 (pageCall)",
		);
	});

	it("does not match a scheme-stripped frame that only contains the own path", () => {
		// Only a whole location token counts: `vendor/assets/chunk-….js` contains
		// the own path but is a different script, so it is page code.
		const stack = "vendor/assets/chunk-CaEYyn41.js:4 (shim)";
		expect(getSourceFromStack(stack, OWN)).toBe(
			"vendor/assets/chunk-CaEYyn41.js:4 (shim)",
		);
	});
});

describe("ownScriptFromStack", () => {
	it("extracts the full own URL and the path under its origin", () => {
		const stack = [
			"Error",
			"    at chrome-extension://ID/assets/chunk-CaEYyn41.js:12:1",
		].join("\n");
		expect(ownScriptFromStack(stack)).toEqual(OWN);
	});

	it("extracts the path from an `at fn (url)` frame (the real module-load shape)", () => {
		const stack = [
			"Error",
			"    at Object.<anonymous> (chrome-extension://ID/assets/chunk-CaEYyn41.js:12:1)",
		].join("\n");
		expect(ownScriptFromStack(stack)).toEqual(OWN);
	});

	it("extracts the URL and path from a Firefox fn@url frame", () => {
		const stack =
			"warn@moz-extension://UUID/contentScript/consoleCapture.js:88:20";
		expect(ownScriptFromStack(stack)).toEqual({
			url: "moz-extension://UUID/contentScript/consoleCapture.js",
			path: "contentScript/consoleCapture.js",
		});
	});

	it("returns undefined when no frame carries a URL", () => {
		expect(ownScriptFromStack("Error")).toBeUndefined();
		expect(ownScriptFromStack(undefined)).toBeUndefined();
	});
});
