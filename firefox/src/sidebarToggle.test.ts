import { describe, expect, it, spyOn } from "bun:test";
import { registerSidebarToggle, type SidebarToggleApi } from "./sidebarToggle";

function harness(options: { toggle?: () => Promise<void> } = {}): {
	api: SidebarToggleApi;
	click: () => void;
	listenerCount: () => number;
	receivers: unknown[];
} {
	const listeners: Array<() => void> = [];
	const receivers: unknown[] = [];
	const toggle = options.toggle ?? (() => Promise.resolve());
	// Method shorthand on purpose: a real sidebarAction.toggle is a method, so
	// `this` must be the sidebarAction object when the listener calls it.
	const sidebarAction = {
		toggle(this: unknown) {
			receivers.push(this);
			return toggle();
		},
	};
	const api: SidebarToggleApi = {
		action: {
			onClicked: {
				addListener(listener) {
					listeners.push(listener);
				},
			},
		},
		sidebarAction,
	};
	return {
		api,
		click: () => {
			for (const listener of listeners) listener();
		},
		listenerCount: () => listeners.length,
		receivers,
	};
}

describe("registerSidebarToggle", () => {
	it("registers a click listener that toggles the sidebar", () => {
		let toggles = 0;
		const { api, click, listenerCount } = harness({
			toggle: () => {
				toggles += 1;
				return Promise.resolve();
			},
		});

		expect(registerSidebarToggle(api)).toBe(true);
		expect(listenerCount()).toBe(1);
		expect(toggles).toBe(0);

		click();
		expect(toggles).toBe(1);
	});

	it("calls toggle as a method on the sidebarAction object", () => {
		const { api, click, receivers } = harness();
		registerSidebarToggle(api);

		click();

		expect(receivers).toHaveLength(1);
		expect(receivers[0]).toBe(api.sidebarAction);
	});

	it("calls toggle synchronously inside the click handler", () => {
		// Firefox only allows sidebarAction.toggle() while the user gesture is
		// live, so an await before the call would lose the permission.
		let toggledSynchronously = false;
		let insideHandler = false;
		const { api, click } = harness({
			toggle: () => {
				toggledSynchronously = insideHandler;
				return Promise.resolve();
			},
		});
		registerSidebarToggle(api);

		insideHandler = true;
		click();
		insideHandler = false;

		expect(toggledSynchronously).toBe(true);
	});

	it("installs only one listener when registered twice on the same event", () => {
		let toggles = 0;
		const { api, click, listenerCount } = harness({
			toggle: () => {
				toggles += 1;
				return Promise.resolve();
			},
		});

		expect(registerSidebarToggle(api)).toBe(true);
		expect(registerSidebarToggle(api)).toBe(true);
		expect(listenerCount()).toBe(1);

		click();
		expect(toggles).toBe(1);
	});

	it("logs and reports false when action.onClicked is unavailable", () => {
		const error = spyOn(console, "error").mockImplementation(() => {});
		expect(
			registerSidebarToggle({ sidebarAction: { toggle: async () => {} } }),
		).toBe(false);
		expect(error).toHaveBeenCalled();
		error.mockRestore();
	});

	it("logs and reports false when sidebarAction.toggle is unavailable", () => {
		const error = spyOn(console, "error").mockImplementation(() => {});
		expect(
			registerSidebarToggle({
				action: { onClicked: { addListener: () => {} } },
			}),
		).toBe(false);
		expect(error).toHaveBeenCalled();
		error.mockRestore();
	});

	it("swallows a rejected toggle and logs it", async () => {
		const warn = spyOn(console, "warn").mockImplementation(() => {});
		const { api, click } = harness({
			toggle: () => Promise.reject(new Error("no sidebar")),
		});
		registerSidebarToggle(api);

		expect(() => click()).not.toThrow();
		await Promise.resolve();
		await Promise.resolve();
		expect(warn).toHaveBeenCalled();
		warn.mockRestore();
	});
});
