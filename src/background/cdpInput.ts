/**
 * Trusted (CDP) input dispatch for click / pressKey / type.
 *
 * On Chrome, `chrome.debugger` lets us synthesize *trusted* input events via
 * the CDP `Input` domain so default browser actions fire (Enter submits forms,
 * clicks pass `event.isTrusted`, focus/selection behave natively). The content
 * script's job is only *preparation* (`prepareClick` / `prepareKeys` internal
 * actions): wait for the element to be actionable, scroll it into view, focus
 * it, and report coordinates / focus state. This module does the rest.
 *
 * Attach/detach follows the existing convention in `handleDebuggerEval`:
 * attach, dispatch, detach in a finally-style path. If attach fails (DevTools
 * open on the tab, or another client attached) the command fails loudly — it
 * does NOT quietly fall back to synthetic events.
 *
 * The CDP `send` and content-script `prepare` steps are injectable so the
 * event-sequence construction can be unit-tested without a real debugger.
 */

import type { Command, CommandResult } from "../types/commands";
import { resolveKey } from "../utils/keyMap";

/** Actions dispatched as trusted CDP input on Chrome. */
export const CDP_INPUT_ACTIONS = new Set<Command["action"]>([
	"click",
	"dblclick",
	"rightclick",
	"pressKey",
	"type",
]);

/** CDP `Input.*` sender. Mirrors the `CdpSend` used by `cdpEval`. */
export type CdpInputSender = (
	method: string,
	params: Record<string, unknown>,
) => Promise<unknown>;

/** Resolves the prepared payload reported by the content script. */
export type PrepareSender = (command: Command) => Promise<{
	x?: number;
	y?: number;
	focused?: boolean;
}>;

export interface CdpDispatchDeps {
	/** Injected CDP sender (test seam). When omitted, `chrome.debugger` is used. */
	send?: CdpInputSender;
	/** Injected prepare sender (test seam). When omitted, a real content-script
	 *  `EXECUTE_COMMAND` message is sent. */
	prepare?: PrepareSender;
}

/**
 * Thrown by the default prepare path when the content script isn't ready to
 * receive the `prepareClick`/`prepareKeys` message, so the caller can inject it
 * and retry.
 */
export class ContentScriptNotReadyError extends Error {
	constructor(message = "content script not ready") {
		super(message);
		this.name = "ContentScriptNotReadyError";
	}
}

// ─── Default (real) implementations ────────────────────────────────

function defaultSend(tabId: number): CdpInputSender {
	return (method, params) =>
		chrome.debugger.sendCommand({ tabId }, method, params);
}

async function defaultPrepare(
	tabId: number,
	command: Command,
): Promise<{ x?: number; y?: number; focused?: boolean }> {
	const result = await new Promise<CommandResult | null>((resolve) => {
		chrome.tabs.sendMessage(
			tabId,
			{ type: "EXECUTE_COMMAND", command },
			(response: CommandResult) => {
				if (chrome.runtime.lastError) {
					resolve(null);
					return;
				}
				resolve(response);
			},
		);
	});
	if (!result) {
		throw new ContentScriptNotReadyError(
			`content script not ready on tab ${tabId}`,
		);
	}
	if (!result.success) {
		throw new Error(result.error ?? "element preparation failed");
	}
	return (result.data ?? {}) as { x?: number; y?: number; focused?: boolean };
}

/**
 * Run `fn` with a CDP sender. When an injected sender is provided (tests or
 * callers that already own the attach), no real attach/detach happens.
 * Otherwise attach to the tab, dispatch, and detach in a finally path — and if
 * attach fails (DevTools open / another client attached) surface the error
 * verbatim rather than silently downgrading to synthetic input.
 */
async function runWithDebugger(
	tabId: number,
	injected: CdpInputSender | undefined,
	fn: (send: CdpInputSender) => Promise<void>,
): Promise<void> {
	if (injected) {
		await fn(injected);
		return;
	}
	const target = { tabId };
	try {
		await chrome.debugger.attach(target, "1.3");
	} catch (err) {
		throw new Error(
			`Failed to attach debugger for trusted input (is DevTools open on this tab, or another debugger client attached?): ${
				err instanceof Error ? err.message : String(err)
			}`,
		);
	}
	try {
		await fn(defaultSend(tabId));
	} finally {
		await chrome.debugger.detach(target);
	}
}

// ─── Dispatchers ───────────────────────────────────────────────────

/**
 * Trusted click. Asks the content script to `prepareClick` (element wait +
 * scroll), then attaches and dispatches `mousePressed` + `mouseReleased` at the
 * reported viewport-center coordinates. `rightclick` uses button "right";
 * `dblclick` uses `clickCount: 2` (single press/release pair per CDP convention).
 */
export async function dispatchCdpClick(
	tabId: number,
	command: Command,
	deps: CdpDispatchDeps = {},
): Promise<CommandResult> {
	const prepare = deps.prepare ?? ((c) => defaultPrepare(tabId, c));
	const coords = await prepare({
		action: "prepareClick",
		id: command.id,
		target: command.target,
		options: command.options,
	});
	if (typeof coords.x !== "number" || typeof coords.y !== "number") {
		throw new Error("prepareClick did not return viewport coordinates");
	}

	const button: "left" | "right" =
		command.action === "rightclick" ? "right" : "left";
	const clickCount = command.action === "dblclick" ? 2 : 1;
	// CDP `buttons` bitmask: 1 = left, 2 = right. (Middle isn't used — only
	// click/dblclick/rightclick reach here.)
	const buttons = button === "right" ? 2 : 1;

	await runWithDebugger(tabId, deps.send, async (send) => {
		await send("Input.dispatchMouseEvent", {
			type: "mousePressed",
			x: coords.x,
			y: coords.y,
			button,
			clickCount,
			buttons,
			modifiers: 0,
		});
		await send("Input.dispatchMouseEvent", {
			type: "mouseReleased",
			x: coords.x,
			y: coords.y,
			button,
			clickCount,
			buttons: 0,
			modifiers: 0,
		});
	});

	return { id: command.id, success: true, data: { x: coords.x, y: coords.y } };
}

/**
 * Trusted key press. `prepareKeys` (element wait + focus), then attaches and
 * dispatches `keyDown` (with `text` for printable keys) + `keyUp`, built from
 * the `resolveKey` descriptor.
 */
export async function dispatchCdpKey(
	tabId: number,
	command: Command,
	deps: CdpDispatchDeps = {},
): Promise<CommandResult> {
	const value = command.value;
	if (typeof value !== "string" || value.length === 0) {
		throw new Error("pressKey requires a non-empty key value");
	}
	const descriptor = resolveKey(value);

	const prepare = deps.prepare ?? ((c) => defaultPrepare(tabId, c));
	await prepare({
		action: "prepareKeys",
		id: command.id,
		target: command.target,
		options: command.options,
	});

	const base = {
		key: descriptor.key,
		code: descriptor.code,
		windowsVirtualKeyCode: descriptor.windowsVirtualKeyCode,
	};

	await runWithDebugger(tabId, deps.send, async (send) => {
		await send("Input.dispatchKeyEvent", {
			type: "keyDown",
			...base,
			...(descriptor.text !== undefined ? { text: descriptor.text } : {}),
		});
		await send("Input.dispatchKeyEvent", {
			type: "keyUp",
			...base,
		});
	});

	return { id: command.id, success: true };
}

/**
 * Trusted type. `prepareKeys` (element wait + focus), then attaches and
 * dispatches a single `Input.insertText` with the whole string — matching how
 * IMEs insert text, so no per-character key events are needed for value entry.
 */
export async function dispatchCdpType(
	tabId: number,
	command: Command,
	deps: CdpDispatchDeps = {},
): Promise<CommandResult> {
	const value = command.value;
	if (typeof value !== "string") {
		throw new Error("type requires a string value");
	}

	const prepare = deps.prepare ?? ((c) => defaultPrepare(tabId, c));
	await prepare({
		action: "prepareKeys",
		id: command.id,
		target: command.target,
		options: command.options,
	});

	await runWithDebugger(tabId, deps.send, async (send) => {
		await send("Input.insertText", { text: value });
	});

	return { id: command.id, success: true };
}

/**
 * Route a command to the correct trusted-input dispatcher by action. Used by
 * both the native-host relay (`sendCommandToTab`) and the WS-path background
 * handler (`CDP_INPUT`).
 */
/**
 * CDP-injected mouse/key events only reach a tab that is actually rendered:
 * on a background tab, `Input.dispatchMouseEvent`/`dispatchKeyEvent` ack
 * successfully but the page never sees the events (verified live —
 * `Input.insertText` works on background tabs, dispatched events do not).
 * So before dispatching trusted input, make the target tab active in its
 * window. The short delay lets the renderer un-throttle before the events
 * are injected.
 */
async function ensureTabActive(tabId: number): Promise<void> {
	const tab = await chrome.tabs.get(tabId);
	if (tab.active) return;
	await chrome.tabs.update(tabId, { active: true });
	await new Promise((r) => setTimeout(r, 150));
}

export async function dispatchCdpInput(
	tabId: number,
	command: Command,
	deps: CdpDispatchDeps = {},
): Promise<CommandResult> {
	// Injected senders (tests) skip activation — there is no real browser.
	if (!deps.send) {
		await ensureTabActive(tabId);
	}
	switch (command.action) {
		case "click":
		case "dblclick":
		case "rightclick":
			return dispatchCdpClick(tabId, command, deps);
		case "pressKey":
			return dispatchCdpKey(tabId, command, deps);
		case "type":
			return dispatchCdpType(tabId, command, deps);
		default:
			throw new Error(
				`dispatchCdpInput: action "${command.action}" is not a CDP input action`,
			);
	}
}
