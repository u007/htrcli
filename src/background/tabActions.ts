/**
 * Tab open and close, run in the background service worker.
 *
 * Two entry points reach these functions and must behave the same:
 *  - the OPEN_TAB / CLOSE_TAB runtime messages, sent by content scripts and the
 *    sidepanel on the tab-routed command path;
 *  - the native relay's tab-less openTab / closeTab commands (htrcli `tabs new`
 *    and `tabs close`), which need no page at all.
 */

/** Longest a tab may take to report "complete" before the caller moves on. */
const TAB_LOAD_TIMEOUT_MS = 15000;

/**
 * Resolves once `tabId` reports status "complete", or after TAB_LOAD_TIMEOUT_MS.
 * A timeout is not an error: the caller proceeds with whatever has loaded.
 */
function waitForLoad(tabId: number): Promise<void> {
	return new Promise<void>((resolve) => {
		const onUpdated = (tid: number, info: chrome.tabs.TabChangeInfo) => {
			if (tid === tabId && info.status === "complete") {
				chrome.tabs.onUpdated.removeListener(onUpdated);
				resolve();
			}
		};
		chrome.tabs.onUpdated.addListener(onUpdated);
		setTimeout(resolve, TAB_LOAD_TIMEOUT_MS);
	});
}

/**
 * Opens `url` in a background tab and waits for it to load. With `sessionData`,
 * the data is written to sessionStorage and the tab navigates to `url` again so
 * the page reads it. Resolves with the new tab's ID.
 */
export async function openTab(
	url: string,
	sessionData?: string,
): Promise<number> {
	const tab = await chrome.tabs.create({ url, active: false });
	if (tab.id == null) {
		throw new Error("Opened tab has no id");
	}
	const tabId = tab.id;
	await waitForLoad(tabId);
	if (sessionData) {
		// Inject sessionStorage into the page (main world)
		await chrome.scripting.executeScript({
			target: { tabId },
			world: "MAIN",
			func: (data: string) => {
				sessionStorage.setItem("eReceiptData", data);
			},
			args: [sessionData],
		});
		// Navigate to the preview URL (re-run in case tab redirected)
		await chrome.tabs.update(tabId, { url });
		await waitForLoad(tabId);
	}
	return tabId;
}

/** Closes tab `tabId`. Rejects when the browser cannot remove it. */
export async function closeTab(tabId: number): Promise<void> {
	await chrome.tabs.remove(tabId);
}
