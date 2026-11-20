import { afterEach, beforeEach, describe, expect, jest, test } from "bun:test";
import "../test/domSetup";
import {
	__resetControlStatusForTests,
	hideControlStatus,
	markControlActive,
	markControlSettled,
	refreshControlStatus,
	setControlStatusSuppressed,
} from "./controlStatusBar";

const HOST_ID = "htrncontrol-control-status";
const AMBER = "rgba(245, 158, 11, 0.85)";
const RED = "rgba(239, 68, 68, 0.85)";

function getHost(): HTMLDivElement {
	const el = document.getElementById(HOST_ID);
	if (!el) throw new Error("control status host not found");
	return el as HTMLDivElement;
}

function getLine(): HTMLDivElement {
	const el = getHost().shadowRoot?.querySelector(".line");
	if (!el) throw new Error("control status line not found");
	return el as HTMLDivElement;
}

describe("controlStatusBar", () => {
	beforeEach(() => {
		jest.useFakeTimers();
		__resetControlStatusForTests();
		document.getElementById(HOST_ID)?.remove();
	});

	afterEach(() => {
		jest.useRealTimers();
	});

	test("creates one isolated host on documentElement", () => {
		markControlActive();
		const host = getHost();

		expect(host.parentElement).toBe(document.documentElement);
		expect(host.getAttribute("data-htrncontrol-ignore")).toBe("true");
		expect(host.getAttribute("aria-hidden")).toBe("true");
		expect(host.style.position).toBe("fixed");
		expect(host.style.height).toBe("2px");
		expect(host.style.pointerEvents).toBe("none");
		expect(host.shadowRoot).not.toBeNull();
		expect(host.style.display).not.toBe("none");

		markControlSettled(true);
		markControlActive();
		expect(document.querySelectorAll(`#${HOST_ID}`)).toHaveLength(1);
	});

	test("shows amber while running and red after a failure", () => {
		markControlActive();
		expect(getLine().style.backgroundColor).toBe(AMBER);

		markControlSettled(false);
		expect(getLine().style.backgroundColor).toBe(RED);
	});

	test("hides after the idle grace following a success", () => {
		markControlActive();
		markControlSettled(true);
		expect(getHost().style.display).not.toBe("none");

		jest.advanceTimersByTime(1499);
		expect(getHost().style.display).not.toBe("none");
		jest.advanceTimersByTime(1);
		expect(getHost().style.display).toBe("none");
	});

	test("holds red longer than the success grace", () => {
		markControlActive();
		markControlSettled(false);

		jest.advanceTimersByTime(1500);
		expect(getHost().style.display).not.toBe("none");
		jest.advanceTimersByTime(1500);
		expect(getHost().style.display).toBe("none");
	});

	test("keeps amber until every in-flight command settles", () => {
		markControlActive();
		markControlActive();
		markControlSettled(true);

		expect(getLine().style.backgroundColor).toBe(AMBER);
		expect(getHost().style.display).not.toBe("none");

		markControlSettled(true);
		jest.advanceTimersByTime(1500);
		expect(getHost().style.display).toBe("none");
	});

	test("a failure in an overlapping batch is still shown", () => {
		markControlActive();
		markControlActive();
		markControlSettled(false);
		markControlSettled(true);

		expect(getLine().style.backgroundColor).toBe(RED);
	});

	test("suppression hides and restores without resetting the timer", () => {
		markControlActive();
		markControlSettled(true);

		setControlStatusSuppressed(true);
		expect(getHost().style.display).toBe("none");

		jest.advanceTimersByTime(1000);
		setControlStatusSuppressed(false);
		expect(getHost().style.display).not.toBe("none");

		jest.advanceTimersByTime(500);
		expect(getHost().style.display).toBe("none");
	});

	test("force-hide clears state and survives a later command", () => {
		markControlActive();
		hideControlStatus();
		expect(getHost().style.display).toBe("none");

		markControlActive();
		expect(getHost().style.display).not.toBe("none");
		expect(getLine().style.backgroundColor).toBe(AMBER);
	});

	test("keeps the line hidden until every overlapping capture restores", () => {
		markControlActive();
		setControlStatusSuppressed(true);
		setControlStatusSuppressed(true);

		setControlStatusSuppressed(false);
		expect(getHost().style.display).toBe("none");

		setControlStatusSuppressed(false);
		expect(getHost().style.display).not.toBe("none");
	});

	test("a lost restore is force-cleared by the watchdog", () => {
		markControlActive();
		setControlStatusSuppressed(true);
		expect(getHost().style.display).toBe("none");

		jest.advanceTimersByTime(5000);
		expect(getHost().style.display).not.toBe("none");
	});

	test("refresh re-renders a line hidden by an external override", () => {
		markControlActive();
		expect(getHost().style.display).not.toBe("none");

		getHost().style.display = "none";
		expect(getHost().style.display).toBe("none");

		refreshControlStatus();
		expect(getHost().style.display).not.toBe("none");
	});

	test("a failing shadow root disables the bar without throwing", () => {
		const spy = jest
			.spyOn(Element.prototype, "attachShadow")
			.mockImplementation(() => {
				throw new Error("boom");
			});

		expect(() => markControlActive()).not.toThrow();
		expect(() => markControlSettled(true)).not.toThrow();
		expect(document.getElementById(HOST_ID)).toBeNull();

		// Disabled means it must not retry the failing DOM work on every command.
		expect(() => markControlActive()).not.toThrow();
		expect(spy).toHaveBeenCalledTimes(1);

		spy.mockRestore();
	});
});
