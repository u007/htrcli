import { type DialogPolicy, resolveDialog } from "../background/dialogPolicy";
import type { DialogEntry } from "../types/recording";

const MESSAGE_SOURCE = "htrncontrol-dialog-override";
// Posted by the isolated-world content script (src/contentScript/index.ts)
// once its relay listener is attached. This script runs at document_start
// in the MAIN world, ahead of the isolated-world script's default timing,
// so early dialog events are buffered here until that ready signal arrives.
const RELAY_READY_TYPE = "HTR_DIALOG_OVERRIDE_RELAY_READY";
const MAX_BUFFERED_BEFORE_READY = 100;

interface DialogOverrideWindow extends Window {
	__htrncontrolDialogOverrideInitialized?: boolean;
}

const overrideWindow = window as DialogOverrideWindow;
let relayReady = false;
// Default: pass-through (call the original dialog) until a policy is armed
// via a DIALOG_POLICY message from the background.
let policy: DialogPolicy | null = null;
const pendingBeforeReady: DialogEntry[] = [];

function sendDialogEntry(entry: DialogEntry): void {
	window.postMessage(
		{ source: MESSAGE_SOURCE, type: "DIALOG_ENTRY", entry },
		"*",
	);
}

function postDialogEntry(entry: DialogEntry): void {
	if (!relayReady) {
		if (pendingBeforeReady.length < MAX_BUFFERED_BEFORE_READY) {
			pendingBeforeReady.push(entry);
		}
		return;
	}
	sendDialogEntry(entry);
}

// Wrap a native dialog method with policy-driven auto-answer logic.
// Uses explicit per-method wrappers below instead of a generic to avoid
// type-system friction with the different window.* signatures.
function wrapAlert(original: typeof window.alert): typeof window.alert {
	return (message?: string) => {
		const text = message !== undefined ? String(message) : "";
		if (policy === null) return original(message);
		const { entry } = resolveDialog(policy, "alert", text);
		postDialogEntry(entry);
	};
}

function wrapConfirm(original: typeof window.confirm): typeof window.confirm {
	return (message?: string) => {
		const text = message !== undefined ? String(message) : "";
		if (policy === null) return original(message);
		const { accept, entry } = resolveDialog(policy, "confirm", text);
		postDialogEntry(entry);
		return accept;
	};
}

function wrapPrompt(original: typeof window.prompt): typeof window.prompt {
	return (message?: string, _default?: string) => {
		const text = message !== undefined ? String(message) : "";
		if (policy === null) return original(message, _default);
		const { accept, promptText, entry } = resolveDialog(policy, "prompt", text);
		postDialogEntry(entry);
		return accept ? (promptText ?? "") : null;
	};
}

if (!overrideWindow.__htrncontrolDialogOverrideInitialized) {
	overrideWindow.__htrncontrolDialogOverrideInitialized = true;

	window.alert = wrapAlert(window.alert.bind(window));
	window.confirm = wrapConfirm(window.confirm.bind(window));
	window.prompt = wrapPrompt(window.prompt.bind(window));

	// Listen for a policy update from the content-script relay.
	window.addEventListener("message", (event: MessageEvent) => {
		if (event.source !== window) return;
		if (
			event.data?.source !== MESSAGE_SOURCE ||
			event.data?.type !== "DIALOG_POLICY"
		)
			return;
		policy = event.data.policy as DialogPolicy | null;
	});

	// Listen for the relay-ready signal from the isolated-world content script.
	window.addEventListener("message", function onReady(event: MessageEvent) {
		if (event.source !== window) return;
		if (
			event.data?.source !== MESSAGE_SOURCE ||
			event.data?.type !== RELAY_READY_TYPE
		)
			return;
		relayReady = true;
		// Flush any entries that were buffered before the relay was attached.
		for (const entry of pendingBeforeReady.splice(0)) {
			sendDialogEntry(entry);
		}
		// Remove the one-shot listener so it doesn't accumulate.
		window.removeEventListener("message", onReady);
	});
}
