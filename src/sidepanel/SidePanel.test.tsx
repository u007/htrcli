import { beforeEach, describe, expect, it } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import SidePanel from "./SidePanel";
import { SIDE_PANEL_TAB_STORAGE_KEY } from "./tabState";

/**
 * Structural tests for the two-view panel. SSR deliberately runs without any
 * effects, so no `chrome.*` stub is needed — this pins the markup/ARIA contract
 * and the remembered-tab default, not the polling behaviour.
 */
function parse(html: string): Document {
	return new DOMParser().parseFromString(html, "text/html");
}

function selectedTab(doc: Document): string | null {
	return (
		doc
			.querySelector('[role="tab"][aria-selected="true"]')
			?.getAttribute("id") ?? null
	);
}

describe("SidePanel two-view switcher", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("renders Recording and Tabs, with Recording selected by default", () => {
		const doc = parse(renderToStaticMarkup(<SidePanel />));

		const tabs = doc.querySelectorAll('[role="tab"]');
		expect(tabs.length).toBe(2);
		expect(selectedTab(doc)).toBe("sidepanel-tab-recording");

		// Both panels are mounted; only the selected one is visible.
		const recording = doc.getElementById("sidepanel-panel-recording");
		const tabList = doc.getElementById("sidepanel-panel-tabs");
		expect(recording?.hasAttribute("hidden")).toBe(false);
		expect(tabList?.hasAttribute("hidden")).toBe(true);
		expect(recording?.getAttribute("role")).toBe("tabpanel");
		expect(recording?.getAttribute("aria-labelledby")).toBe(
			"sidepanel-tab-recording",
		);
	});

	it("restores the remembered Tabs view", () => {
		localStorage.setItem(SIDE_PANEL_TAB_STORAGE_KEY, "tabs");

		const doc = parse(renderToStaticMarkup(<SidePanel />));

		expect(selectedTab(doc)).toBe("sidepanel-tab-tabs");
		expect(
			doc.getElementById("sidepanel-panel-recording")?.hasAttribute("hidden"),
		).toBe(true);
		expect(
			doc.getElementById("sidepanel-panel-tabs")?.hasAttribute("hidden"),
		).toBe(false);
	});

	it("falls back to Recording for an unknown remembered value", () => {
		localStorage.setItem(SIDE_PANEL_TAB_STORAGE_KEY, "bogus");
		const doc = parse(renderToStaticMarkup(<SidePanel />));
		expect(selectedTab(doc)).toBe("sidepanel-tab-recording");
	});
});
