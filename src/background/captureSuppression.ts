/**
 * Capture Suppression
 *
 * The control status line is composited into `captureVisibleTab` pixels, so it
 * would otherwise land in screenshot and PDF evidence. These helpers hide it
 * around a capture and restore it afterwards, tolerating tabs with no content
 * script (e.g. browser-internal pages).
 */

import type { ControlStatusSuppressMessage } from "../types/recording";

type SuppressionSender = (
	tabId: number,
	message: ControlStatusSuppressMessage,
) => Promise<unknown>;

const SEND_TIMEOUT_MS = 500;

const defaultSender: SuppressionSender = (tabId, message) =>
	chrome.tabs.sendMessage(tabId, message);

/** A send with no receiver is expected (no content script); it is not an error. */
function isNoReceiverError(error: unknown): boolean {
	const message = error instanceof Error ? error.message : String(error);
	return (
		message.includes("Receiving end does not exist") ||
		message.includes("Could not establish connection") ||
		message.includes("no receiver")
	);
}

/**
 * Bound one send. A content script from an older version may never answer, and
 * every capture in that tab would otherwise wait forever.
 */
function withSendTimeout(work: Promise<unknown>): Promise<unknown> {
	return new Promise((resolve, reject) => {
		const timer = setTimeout(
			() => reject(new Error("control-status message timed out")),
			SEND_TIMEOUT_MS,
		);
		work.then(
			(value) => {
				clearTimeout(timer);
				resolve(value);
			},
			(error) => {
				clearTimeout(timer);
				reject(error);
			},
		);
	});
}

/**
 * Tell a tab's content script to hide or restore the status line. A tab without
 * a content script must never block a capture, so that failure is swallowed;
 * any other failure is logged so a hang or a version mismatch leaves a trace.
 */
export async function setControlStatusSuppressedInTab(
	tabId: number,
	suppressed: boolean,
	send: SuppressionSender = defaultSender,
): Promise<void> {
	try {
		await withSendTimeout(
			send(tabId, { type: "CONTROL_STATUS_SUPPRESS", suppressed }),
		);
	} catch (error) {
		if (!isNoReceiverError(error)) {
			console.warn(
				"[HTR NControl] control-status suppression failed:",
				error instanceof Error ? error.message : error,
			);
		}
	}
}

/**
 * Run a capture with the status line hidden. The restore runs in `finally`, so
 * a failed capture still leaves the indicator in its normal state.
 */
export async function withControlStatusSuppressed<T>(
	tabId: number,
	capture: () => Promise<T>,
	send: SuppressionSender = defaultSender,
): Promise<T> {
	await setControlStatusSuppressedInTab(tabId, true, send);
	try {
		return await capture();
	} finally {
		await setControlStatusSuppressedInTab(tabId, false, send);
	}
}
