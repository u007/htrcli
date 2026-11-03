/**
 * Pure helpers for the side panel's two-tab switcher.
 *
 * The panel has two top-level views — Recording and Tabs — and remembers the
 * last one in `localStorage` so reopening the panel (Chrome side panel /
 * Firefox sidebar) returns the user to where they were. Kept as pure functions
 * so the parsing/persistence contract is testable without rendering React.
 */

export const SIDE_PANEL_TABS = ["recording", "tabs"] as const;

export type SidePanelTab = (typeof SIDE_PANEL_TABS)[number];

export const SIDE_PANEL_TAB_STORAGE_KEY = "htr.sidepanel.tab";

/** Unknown or corrupt stored values fall back to the recording view. */
export function parseSidePanelTab(value: unknown): SidePanelTab {
	return value === "tabs" ? "tabs" : "recording";
}

/**
 * Reads the remembered tab, tolerating a missing or blocked storage. The storage
 * is resolved inside the try: `globalThis.localStorage` can THROW on access in a
 * document with a denied storage partition, and a default parameter would throw
 * before this catch could run.
 */
export function readSidePanelTab(
	storage?: Pick<Storage, "getItem">,
): SidePanelTab {
	try {
		const target = storage === undefined ? globalThis.localStorage : storage;
		return parseSidePanelTab(target.getItem(SIDE_PANEL_TAB_STORAGE_KEY));
	} catch (error) {
		// Storage can throw in private/sandboxed contexts; the default tab is
		// still perfectly usable, so this is logged, not surfaced.
		console.warn("[HTR NControl] reading side panel tab failed:", error);
		return "recording";
	}
}

/** Remembers the active tab. A storage failure must never break the panel. */
export function writeSidePanelTab(
	tab: SidePanelTab,
	storage?: Pick<Storage, "setItem">,
): void {
	try {
		const target = storage === undefined ? globalThis.localStorage : storage;
		target.setItem(SIDE_PANEL_TAB_STORAGE_KEY, tab);
	} catch (error) {
		console.warn("[HTR NControl] saving side panel tab failed:", error);
	}
}

/** Arrow/Home/End navigation for `role="tablist"` (roving tabindex). */
export function nextSidePanelTab(
	current: SidePanelTab,
	key: string,
): SidePanelTab | null {
	const index = SIDE_PANEL_TABS.indexOf(current);
	if (key === "ArrowRight") {
		return SIDE_PANEL_TABS[(index + 1) % SIDE_PANEL_TABS.length];
	}
	if (key === "ArrowLeft") {
		return SIDE_PANEL_TABS[
			(index - 1 + SIDE_PANEL_TABS.length) % SIDE_PANEL_TABS.length
		];
	}
	if (key === "Home") return SIDE_PANEL_TABS[0];
	if (key === "End") return SIDE_PANEL_TABS[SIDE_PANEL_TABS.length - 1];
	return null;
}
