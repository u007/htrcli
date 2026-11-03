import { describe, expect, it } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import {
	ConnectedTabs,
	type ConnectedTabsState,
	type TabInfo,
} from "./ConnectedTabs";

/**
 * SSR runs no effects, so the list is driven entirely by the `tabs` prop passed
 * down from the side panel root. This pins that contract: rows come from the
 * prop, and the panel has no collapsible header.
 */
function parse(html: string): Document {
	return new DOMParser().parseFromString(html, "text/html");
}

function stateWith(tabs: TabInfo[]): ConnectedTabsState {
	return {
		tabs,
		setTabs: () => {},
		refresh: async () => {},
	};
}

const TABS: TabInfo[] = [
	{ id: 1, url: "https://a.test", title: "Alpha", active: true, index: 0 },
	{ id: 2, url: "https://b.test", title: "Beta", active: false, index: 1 },
];

describe("ConnectedTabs", () => {
	it("renders one row per tab from the shared tabs prop", () => {
		const doc = parse(
			renderToStaticMarkup(<ConnectedTabs {...stateWith(TABS)} />),
		);

		const rows = doc.querySelectorAll(".ct-item");
		expect(rows.length).toBe(2);
		expect(rows[0]?.querySelector(".ct-name")?.textContent).toBe("Alpha");
		expect(rows[1]?.querySelector(".ct-name")?.textContent).toBe("Beta");
		expect(rows[0]?.querySelector(".ct-badge")?.textContent).toBe("active");
	});

	it("has no collapsible header; the list is the only layout", () => {
		const doc = parse(
			renderToStaticMarkup(<ConnectedTabs {...stateWith(TABS)} />),
		);

		expect(doc.querySelector(".ct-header")).toBeNull();
		expect(doc.querySelector(".ct-list")).not.toBeNull();
	});

	it("shows the empty state when no tabs are connected", () => {
		const doc = parse(
			renderToStaticMarkup(<ConnectedTabs {...stateWith([])} />),
		);

		expect(doc.querySelectorAll(".ct-item").length).toBe(0);
		expect(doc.querySelector(".ct-empty")?.textContent).toContain(
			"No tabs connected.",
		);
	});
});
