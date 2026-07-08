/**
 * Per-realm DOM test setup.
 *
 * `bun test` runs each test file in its own realm, and happy-dom's
 * `GlobalRegistrator` (wired via `bunfig.toml` `[test] preload`) re-registers a
 * fresh `Window` per realm. Any global mutations performed in the preload do NOT
 * survive into the per-file realm, so the viewport/rect mocks below must be
 * applied from inside the test file's own realm — i.e. by importing this module.
 *
 * Importing this module from a DOM-backed test file:
 *   1. ensures happy-dom is registered, and
 *   2. installs the non-zero `getBoundingClientRect` and `innerWidth`/`innerHeight`
 *      stubs the element finder's `isVisible()` relies on.
 */
import { GlobalRegistrator } from "@happy-dom/global-registrator";

// Register happy-dom if not already (the preload may or may not have run in this
// realm). `register()` throws if called twice, so guard against that.
try {
	GlobalRegistrator.register();
} catch {
	// Already registered in this realm — fine.
}

// happy-dom does not perform layout, so `getBoundingClientRect()` returns a
// zero-size rect. `isVisible()` treats a zero-size rect as "not visible", which
// would make every DOM test fail actionability checks. Provide a stable non-zero
// default rect so visibility logic can be exercised.
if (typeof Element !== "undefined") {
	const rect = {
		x: 0,
		y: 0,
		width: 100,
		height: 50,
		top: 0,
		bottom: 50,
		left: 0,
		right: 100,
		toJSON() {},
	};
	Element.prototype.getBoundingClientRect = () => rect as DOMRect;
}

// happy-dom's `window.innerWidth`/`innerHeight` getters read
// `browserFrame.page.viewport`, which can be null in some execution contexts and
// throw on every visibility check. Replace the getters with plain data properties
// so `isVisible()` never touches the (possibly null) page.
if (typeof window !== "undefined") {
	Object.defineProperty(window, "innerWidth", {
		value: 1024,
		configurable: true,
	});
	Object.defineProperty(window, "innerHeight", {
		value: 768,
		configurable: true,
	});
}
