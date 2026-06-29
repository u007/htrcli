// Firefox build polyfill entry.
//
// In Firefox, the `chrome` global does not exist (only `browser` does),
// but the shared source code uses `chrome.*` everywhere. The
// `webextension-polyfill` package ships a UMD bundle that, when
// evaluated, exposes a global `browser` and (in Chrome) also sets
// `chrome`. We side-effect-import that bundle so `globalThis.browser`
// is populated, then assign it to `globalThis.chrome` so all existing
// `chrome.tabs`, `chrome.runtime`, `chrome.storage`, etc. calls
// resolve to the same API.
//
// Importing this file at the top of every Firefox-specific entry
// point (background, content script, page bundles) makes the rest of
// the shared source code work without modification.

import "webextension-polyfill";

// We use a small `unknown` cast and a targeted property access here
// rather than `any` so TypeScript keeps narrowing the rest of the
// file normally. The polyfill only writes `chrome` and `browser` on
// `globalThis`, so we only need to know that those keys may be set.
type MaybePolyfilledGlobals = typeof globalThis & {
	browser?: typeof chrome;
	chrome?: typeof chrome;
};
const g = globalThis as MaybePolyfilledGlobals;
if (g.browser && !g.chrome) {
	g.chrome = g.browser;
}
