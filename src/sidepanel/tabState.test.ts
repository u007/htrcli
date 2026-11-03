import { describe, expect, it, spyOn } from "bun:test";
import {
	nextSidePanelTab,
	parseSidePanelTab,
	readSidePanelTab,
	SIDE_PANEL_TAB_STORAGE_KEY,
	writeSidePanelTab,
} from "./tabState";

function fakeStorage(
	initial: Record<string, string> = {},
	fail: { get?: boolean; set?: boolean } = {},
) {
	const values = new Map(Object.entries(initial));
	return {
		getItem(key: string): string | null {
			if (fail.get) throw new Error("storage blocked");
			return values.get(key) ?? null;
		},
		setItem(key: string, value: string): void {
			if (fail.set) throw new Error("storage blocked");
			values.set(key, value);
		},
		/** Test helper: the raw stored value, bypassing getItem. */
		peek(key: string): string | null {
			return values.get(key) ?? null;
		},
	};
}

describe("parseSidePanelTab", () => {
	it("accepts the two known views", () => {
		expect(parseSidePanelTab("recording")).toBe("recording");
		expect(parseSidePanelTab("tabs")).toBe("tabs");
	});

	it("falls back to Recording for missing or corrupt values", () => {
		for (const value of [undefined, null, "", "tabs ", "Tabs", "other", 1]) {
			expect(parseSidePanelTab(value)).toBe("recording");
		}
	});
});

describe("readSidePanelTab", () => {
	it("returns the remembered tab", () => {
		const storage = fakeStorage({ [SIDE_PANEL_TAB_STORAGE_KEY]: "tabs" });
		expect(readSidePanelTab(storage)).toBe("tabs");
	});

	it("defaults to Recording when nothing is stored", () => {
		expect(readSidePanelTab(fakeStorage())).toBe("recording");
	});

	it("defaults to Recording when storage is unavailable or throws", () => {
		const warn = spyOn(console, "warn").mockImplementation(() => {});
		expect(readSidePanelTab(fakeStorage({}, { get: true }))).toBe("recording");
		expect(readSidePanelTab(undefined)).toBe("recording");
		expect(warn).toHaveBeenCalled();
		warn.mockRestore();
	});

	it("survives a localStorage getter that throws on access", () => {
		// Regression: the storage used to be resolved as a DEFAULT PARAMETER, so
		// `globalThis.localStorage` throwing escaped the try/catch and blanked the
		// panel before it could render (seen in headless Chromium, where an
		// extension page can be denied a storage partition).
		const warn = spyOn(console, "warn").mockImplementation(() => {});
		const original = Object.getOwnPropertyDescriptor(
			globalThis,
			"localStorage",
		);
		Object.defineProperty(globalThis, "localStorage", {
			configurable: true,
			get() {
				throw new Error("access denied");
			},
		});
		try {
			expect(readSidePanelTab()).toBe("recording");
			expect(() => writeSidePanelTab("tabs")).not.toThrow();
			expect(warn).toHaveBeenCalled();
		} finally {
			if (original) {
				Object.defineProperty(globalThis, "localStorage", original);
			} else {
				Reflect.deleteProperty(globalThis, "localStorage");
			}
			warn.mockRestore();
		}
	});
});

describe("writeSidePanelTab", () => {
	it("stores the chosen tab under the stable key", () => {
		const storage = fakeStorage();
		writeSidePanelTab("tabs", storage);
		expect(storage.peek(SIDE_PANEL_TAB_STORAGE_KEY)).toBe("tabs");
	});

	it("never throws when storage is blocked", () => {
		const warn = spyOn(console, "warn").mockImplementation(() => {});
		expect(() =>
			writeSidePanelTab("tabs", fakeStorage({}, { set: true })),
		).not.toThrow();
		expect(() => writeSidePanelTab("tabs", undefined)).not.toThrow();
		expect(warn).toHaveBeenCalled();
		warn.mockRestore();
	});
});

describe("nextSidePanelTab", () => {
	it("moves right and left with wraparound", () => {
		expect(nextSidePanelTab("recording", "ArrowRight")).toBe("tabs");
		expect(nextSidePanelTab("tabs", "ArrowRight")).toBe("recording");
		expect(nextSidePanelTab("tabs", "ArrowLeft")).toBe("recording");
		expect(nextSidePanelTab("recording", "ArrowLeft")).toBe("tabs");
	});

	it("supports Home and End", () => {
		expect(nextSidePanelTab("tabs", "Home")).toBe("recording");
		expect(nextSidePanelTab("recording", "End")).toBe("tabs");
	});

	it("ignores unrelated keys so typing is not hijacked", () => {
		for (const key of ["Enter", " ", "ArrowUp", "ArrowDown", "a", "Tab"]) {
			expect(nextSidePanelTab("recording", key)).toBeNull();
		}
	});
});
