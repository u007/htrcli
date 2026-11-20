import { beforeEach, describe, expect, test } from "bun:test";
import "../test/domSetup";
import { __resetControlStatusForTests } from "./controlStatusBar";

type Listener = (
	message: unknown,
	sender: unknown,
	sendResponse: (response?: unknown) => void,
) => boolean | undefined;

const listeners: Listener[] = [];
const HOST_ID = "htrncontrol-control-status";

// Minimal chrome surface the content script touches at import time and while
// handling a command. Installed before importing the entry point.
(globalThis as { chrome?: unknown }).chrome = {
	runtime: {
		onMessage: {
			addListener: (listener: Listener) => {
				listeners.push(listener);
			},
			removeListener: (listener: Listener) => {
				const index = listeners.indexOf(listener);
				if (index >= 0) listeners.splice(index, 1);
			},
		},
		sendMessage: async (message: { type?: string }) => {
			if (message?.type === "GET_CURRENT_TAB_ID") return { tabId: 1 };
			if (message?.type === "CAPTURE_SCREENSHOT") return {};
			return undefined;
		},
	},
};

await import("./index");

function dispatch(
	message: unknown,
	sendResponse: (response?: unknown) => void = () => {},
): void {
	// Dispatch to every registered listener; only the entry point acts on
	// EXECUTE_COMMAND, so ordering cannot make the test flaky.
	for (const listener of listeners) listener(message, {}, sendResponse);
}

describe("content-script wiring", () => {
	beforeEach(() => {
		__resetControlStatusForTests();
		document.getElementById(HOST_ID)?.remove();
	});

	test("EXECUTE_COMMAND shows the bar and settles on failure", async () => {
		dispatch({
			type: "EXECUTE_COMMAND",
			command: { id: "c1", action: "no-such-action" },
		});
		await new Promise((resolve) => setTimeout(resolve, 0));

		const host = document.getElementById(HOST_ID);
		expect(host).not.toBeNull();
		expect(host?.style.display).not.toBe("none");
	});

	test("control-status refresh re-renders the bar from state", async () => {
		dispatch({
			type: "EXECUTE_COMMAND",
			command: { id: "c3", action: "no-such-action" },
		});
		await new Promise((resolve) => setTimeout(resolve, 0));

		const host = document.getElementById(HOST_ID);
		if (!host) throw new Error("control status host not found");
		host.style.display = "none";

		window.dispatchEvent(new Event("htrncontrol:control-status-refresh"));
		expect(host.style.display).not.toBe("none");
	});

	test("CONTROL_STATUS_SUPPRESS hides and restores the bar", () => {
		dispatch({
			type: "EXECUTE_COMMAND",
			command: { id: "c2", action: "no-such-action" },
		});
		expect(document.getElementById(HOST_ID)).not.toBeNull();

		const responses: unknown[] = [];
		dispatch(
			{ type: "CONTROL_STATUS_SUPPRESS", suppressed: true },
			(response) => responses.push(response),
		);
		expect(document.getElementById(HOST_ID)?.style.display).toBe("none");

		dispatch(
			{ type: "CONTROL_STATUS_SUPPRESS", suppressed: false },
			(response) => responses.push(response),
		);
		expect(document.getElementById(HOST_ID)?.style.display).not.toBe("none");
	});
});
