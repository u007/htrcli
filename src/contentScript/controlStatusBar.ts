/**
 * Control Status Bar
 *
 * A 2px line pinned to the top of the page while a remote-control command is
 * executing. The line lives inside a shadow root so host-page CSS cannot
 * restyle it and its styles cannot leak into the page.
 *
 * Status colors: amber = command running, red = last command failed.
 * A successful command keeps the amber line until the idle timer fires.
 */

const HOST_ID = "htrncontrol-control-status";
const IDLE_DELAY_MS = 1500;
const FAILED_HOLD_MS = 3000;
// A capture whose restore never arrives must not hide the indicator forever.
const SUPPRESS_WATCHDOG_MS = 5000;

const LINE_COLORS = {
	running: "rgba(245, 158, 11, 0.85)",
	failed: "rgba(239, 68, 68, 0.85)",
} as const;

const SHADOW_CSS = `
	.line {
		width: 100%;
		height: 100%;
		transition: background-color 120ms linear;
	}
	@media (prefers-reduced-motion: reduce) {
		.line { transition: none; }
	}
`;

type ControlStatus = "running" | "failed" | "idle";

let host: HTMLDivElement | null = null;
let line: HTMLDivElement | null = null;
let inFlight = 0;
let status: ControlStatus = "idle";
// A failure anywhere in an overlapping batch must still be visible when the
// last command settles.
let sawFailure = false;
let idleTimer: ReturnType<typeof setTimeout> | null = null;
let suppressDepth = 0;
let suppressWatchdog: ReturnType<typeof setTimeout> | null = null;
let disabled = false;

/**
 * Create the host + shadow root once. Safe at document_start, before body
 * exists: the host attaches to documentElement, which is already present.
 * A UI failure must never break remote control, so errors disable the bar.
 */
function ensureBar(): void {
	if (disabled || host?.isConnected) return;
	if (!document.documentElement) return;

	try {
		host = document.createElement("div");
		host.id = HOST_ID;
		host.setAttribute("data-htrncontrol-ignore", "true");
		host.setAttribute("aria-hidden", "true");
		Object.assign(host.style, {
			position: "fixed",
			top: "0",
			left: "0",
			right: "0",
			height: "2px",
			zIndex: "2147483647",
			pointerEvents: "none",
		});
		host.style.display = "none";

		const shadow = host.attachShadow({ mode: "open" });
		const style = document.createElement("style");
		style.textContent = SHADOW_CSS;
		line = document.createElement("div");
		line.className = "line";
		shadow.append(style, line);

		document.documentElement.appendChild(host);
	} catch (error) {
		disabled = true;
		console.warn("[HTR NControl] Control status bar unavailable:", error);
	}
}

function clearTimer(): void {
	if (idleTimer !== null) {
		clearTimeout(idleTimer);
		idleTimer = null;
	}
}

function render(): void {
	if (!host || !line) return;
	const visible = suppressDepth === 0 && status !== "idle";
	host.style.display = visible ? "" : "none";
	if (status !== "idle") {
		line.style.backgroundColor =
			status === "failed" ? LINE_COLORS.failed : LINE_COLORS.running;
	}
}

function armHide(ms: number): void {
	clearTimer();
	idleTimer = setTimeout(() => {
		status = "idle";
		idleTimer = null;
		render();
	}, ms);
}

/** Mark a command as started. Amber stays up until all in-flight commands settle. */
export function markControlActive(): void {
	ensureBar();
	if (inFlight === 0) sawFailure = false;
	inFlight += 1;
	clearTimer();
	status = "running";
	render();
}

/**
 * Mark a command as finished. On success the amber line stays for a short idle
 * grace (avoids flicker between commands); on failure it turns red and holds
 * longer so the error is visible.
 */
export function markControlSettled(success: boolean): void {
	inFlight = Math.max(0, inFlight - 1);
	if (!success) sawFailure = true;
	if (inFlight > 0) return;

	if (sawFailure) {
		status = "failed";
		armHide(FAILED_HOLD_MS);
	} else {
		status = "running";
		armHide(IDLE_DELAY_MS);
	}
	render();
}

/**
 * Hide the line while evidence is captured. Refcounted: overlapping captures
 * each hold one level, and a watchdog force-clears a lost restore so a dropped
 * message cannot leave the indicator hidden for the tab's lifetime.
 *
 * Suppression deliberately does not touch the idle timer, so pausing for a
 * screenshot cannot race the hide countdown.
 */
export function setControlStatusSuppressed(value: boolean): void {
	if (value) {
		suppressDepth += 1;
		if (suppressWatchdog !== null) clearTimeout(suppressWatchdog);
		suppressWatchdog = setTimeout(() => {
			suppressDepth = 0;
			suppressWatchdog = null;
			render();
		}, SUPPRESS_WATCHDOG_MS);
	} else if (suppressDepth > 0) {
		suppressDepth -= 1;
		if (suppressDepth === 0 && suppressWatchdog !== null) {
			clearTimeout(suppressWatchdog);
			suppressWatchdog = null;
		}
	}
	render();
}

/** Re-render from current state, e.g. after an external display override ends. */
export function refreshControlStatus(): void {
	render();
}

/** Force the line back to hidden, e.g. when control is explicitly torn down. */
export function hideControlStatus(): void {
	clearTimer();
	if (suppressWatchdog !== null) {
		clearTimeout(suppressWatchdog);
		suppressWatchdog = null;
	}
	inFlight = 0;
	suppressDepth = 0;
	sawFailure = false;
	status = "idle";
	render();
}

/**
 * Test seam: reset every module-level field, including the sticky `disabled`
 * flag, so one test's forced failure cannot leak into the next.
 */
export function __resetControlStatusForTests(): void {
	clearTimer();
	if (suppressWatchdog !== null) {
		clearTimeout(suppressWatchdog);
		suppressWatchdog = null;
	}
	host = null;
	line = null;
	inFlight = 0;
	suppressDepth = 0;
	sawFailure = false;
	status = "idle";
	disabled = false;
}
