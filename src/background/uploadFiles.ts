/**
 * Upload File handler for the extension transport.
 *
 * Uses chrome.debugger's DOM.* API to resolve a CSS selector to a DOM nodeId,
 * then calls DOM.setFileInputFiles to set the file input's value without an
 * OS file-picker. Firefox (no chrome.debugger) gets an explicit unsupported
 * error rather than a silent no-op.
 *
 * The `send` function is injected (test seam) so no real debugger connection
 * is needed in unit tests.
 */

type CDPSend = (
	method: string,
	params: Record<string, unknown>,
) => Promise<Record<string, unknown>>;

/**
 * Resolve a CSS selector to a DOM nodeId via DOM.getDocument + DOM.querySelector,
 * then set the file input's files via DOM.setFileInputFiles.
 */
export async function resolveAndSetFiles(
	send: CDPSend,
	selector: string,
	files: string[],
): Promise<void> {
	const doc = (await send("DOM.getDocument", {
		depth: 0,
	})) as { root: { nodeId: number } };

	const qs = (await send("DOM.querySelector", {
		nodeId: doc.root.nodeId,
		selector,
	})) as { nodeId: number };

	if (!qs.nodeId) {
		throw new Error(`no element matched CSS selector "${selector}"`);
	}

	await send("DOM.setFileInputFiles", { nodeId: qs.nodeId, files });
}
