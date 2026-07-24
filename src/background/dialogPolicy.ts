/**
 * Pure decision logic for answering a JavaScript dialog per an armed policy.
 * Shared by the Chrome CDP path (Page.handleJavaScriptDialog) and the Firefox
 * MAIN-world override, so "accept/dismiss/respond" is defined in one place.
 */

import type { DialogEntry } from "../types/recording";

export type DialogAction = "accept" | "dismiss" | "respond";

export interface DialogPolicy {
	action: DialogAction;
	text?: string;
}

export function resolveDialog(
	policy: DialogPolicy,
	dialogType: DialogEntry["dialogType"],
	message: string,
): { accept: boolean; promptText?: string; entry: DialogEntry } {
	const accept = policy.action === "accept" || policy.action === "respond";
	const entry: DialogEntry = {
		dialogType,
		message,
		resolvedAction: accept ? "accept" : "dismiss",
	};
	if (policy.action === "respond") {
		const text = policy.text ?? "";
		entry.respondedText = text;
		return { accept, promptText: text, entry };
	}
	return { accept, entry };
}
