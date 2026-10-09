/**
 * Stack-attribution helpers for the MAIN-world console wrapper.
 *
 * Pure and side-effect free: consoleCapture.ts imports this, and so do the
 * unit tests (importing consoleCapture itself would wrap the test runner's
 * console). The one rule this module exists to enforce: the extension's own
 * wrapper frame must NEVER be reported as a console entry's source.
 */

/** The extension script that built the wrapper; see ownScriptFromStack. */
export interface OwnScript {
	url: string;
	path: string;
}

// The wrapper always runs from one of these — this is the primary own-frame
// signal and works regardless of how the bundle is named or chunked.
const EXTENSION_SCHEMES = [
	"chrome-extension://",
	"moz-extension://",
	"safari-web-extension://",
] as const;

// Any `scheme://…`: recognises frames and (after the own-frame checks) proves
// a frame points at ordinary page code.
const SCHEME_RE = /[a-z][a-z0-9+.-]*:\/\//i;
// Scheme-less frames count only as `path/file.ext:line` (`assets/chunk-X.js:2`)
// — bare `foo:1` prose and minified code must not be mistaken for frames.
const PATH_LINE_RE = /(?:[\w.-]+\/)+[\w.-]+\.[a-z]{1,6}:\d+/i;
// A URL token inside a frame, stopping at whitespace or a closing paren.
const URL_RE = /[a-z][a-z0-9+.-]*:\/\/[^\s)]+/i;
// V8's frame for code without a script URL (DevTools console input,
// `new Function`, inline eval): `<anonymous>:line:col`. The wrapper is a real
// script, so such a frame is always page code.
const ANON_RE = /<anonymous>:\d+:\d+/;

function isFrameLine(line: string): boolean {
	return SCHEME_RE.test(line) || ANON_RE.test(line) || PATH_LINE_RE.test(line);
}

/** Location tokens of a frame line, with their `:line[:col]` suffix removed. */
function locationTokens(line: string): string[] {
	return (line.match(/[^\s()@]+/g) ?? []).map((token) =>
		token.replace(/(:\d+){1,2}$/, ""),
	);
}

function isOwnFrame(line: string, ownScript?: OwnScript): boolean {
	for (const scheme of EXTENSION_SCHEMES) {
		if (line.includes(scheme)) return true;
	}
	// Dev-build filename (the original filter): catches a wrapper served from
	// a non-extension origin when the own script could not be captured.
	if (line.includes("consoleCapture")) return true;
	if (ownScript === undefined) return false;
	// A frame that still carries its scheme must contain the full own URL. A
	// bare path (`index.js`) would also match any page URL ending in it.
	if (line.includes(ownScript.url)) return true;
	// A scheme-stripped frame keeps only part of the URL: it must equal a whole
	// location token in one of those forms, not merely contain the path.
	const forms = ownLocations(ownScript);
	return locationTokens(line).some((token) => forms.includes(token));
}

/**
 * The forms a scheme-stripped frame can keep of the own URL: the bare path,
 * the path with a leading slash, and host + path (`ID/assets/x.js`).
 */
function ownLocations(ownScript: OwnScript): string[] {
	const hostAndPath = ownScript.url.slice(ownScript.url.indexOf("://") + 3);
	return [ownScript.path, `/${ownScript.path}`, hostAndPath];
}

/**
 * Return the first PAGE frame of a console-call stack, or `undefined`.
 *
 * Walks every line instead of slicing a fixed header/wrapper prefix: pages
 * may replace Error.prepareStackTrace, yielding headerless, scheme-stripped,
 * or even non-string stacks. Own frames (extension scheme, dev filename, or
 * the captured own path) are skipped; prose/header/line-number/code lines are
 * ignored. A surviving frame is page code when it carries a scheme or is an
 * `<anonymous>` frame, or when it is scheme-stripped but the own path IS
 * known (it was checked and did not match). A scheme-stripped frame with the
 * own path unknown is neither provably ours nor provably page code and aborts
 * the search — reporting nothing is safer than reporting the wrapper.
 */
export function getSourceFromStack(
	stack: unknown,
	ownScript?: OwnScript,
): string | undefined {
	if (typeof stack !== "string") return undefined;
	if (stack.length === 0) return undefined;
	for (const rawLine of stack.split("\n")) {
		const line = rawLine.trim();
		if (line.length === 0) continue;
		if (!isFrameLine(line)) continue;
		if (isOwnFrame(line, ownScript)) continue;
		// Own checks ran first, so a scheme, an <anonymous> frame, or a
		// scheme-stripped frame that failed a KNOWN own path is page code.
		if (SCHEME_RE.test(line) || ANON_RE.test(line) || ownScript !== undefined) {
			return line.replace(/^at\s+/, "");
		}
		return undefined;
	}
	return undefined;
}

/**
 * The extension script that built the stack: its full URL (scheme, origin and
 * path, no line/column) and the path under its origin, which is what a frame
 * keeps once a custom prepareStackTrace strips the scheme. Call at module load
 * in consoleCapture — document_start, before page code can replace
 * Error.prepareStackTrace — and pass the result to getSourceFromStack. Returns
 * `undefined` when no frame carries a usable URL; callers then rely on the
 * scheme check alone.
 */
export function ownScriptFromStack(stack: unknown): OwnScript | undefined {
	if (typeof stack !== "string") return undefined;
	for (const rawLine of stack.split("\n")) {
		const match = URL_RE.exec(rawLine);
		if (match === null) continue;
		const url = match[0].replace(/(:\d+){1,2}$/, "");
		const schemeEnd = url.indexOf("://") + 3;
		const pathStart = url.indexOf("/", schemeEnd);
		if (pathStart === -1) continue;
		const path = url.slice(pathStart + 1);
		if (path.length > 0) return { url, path };
	}
	return undefined;
}
