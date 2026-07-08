import { describe, expect, it } from "bun:test";
import {
	type TabUpdateListener,
	type TabWatcherDeps,
	waitForTabComplete,
	watchForTriggeredNavigation,
} from "./nativeHost";

/**
 * Build injectable `chrome.tabs.onUpdated` deps that record attached listeners
 * and let the test fire synthetic tab-updated events.
 *
 * Tests pass minimal objects — only `status` and `url` are populated. The
 * `TabUpdateListener` type uses the full `chrome.tabs.TabChangeInfo` /
 * `chrome.tabs.Tab` shapes; the runtime only reads `status` and `url`, so
 * a `Partial` cast is enough for tests.
 */
function makeDeps() {
	const listeners: TabUpdateListener[] = [];
	const deps: TabWatcherDeps = {
		addListener: (fn) => {
			listeners.push(fn);
		},
		removeListener: (fn) => {
			const i = listeners.indexOf(fn);
			if (i >= 0) listeners.splice(i, 1);
		},
	};
	const fire = (
		tabId: number,
		changeInfo: Partial<chrome.tabs.TabChangeInfo> = {},
		tab: Partial<chrome.tabs.Tab> = { status: changeInfo.status },
	) => {
		// Copy so a listener removed mid-dispatch doesn't skip others.
		for (const l of [...listeners]) {
			l(tabId, changeInfo as chrome.tabs.TabChangeInfo, tab as chrome.tabs.Tab);
		}
	};
	return { deps, fire, listeners };
}

describe("watchForTriggeredNavigation", () => {
	it("resolves 'none' promptly when no navigation occurs in the window", async () => {
		const { deps } = makeDeps();
		const watcher = watchForTriggeredNavigation(1, deps, 1000);
		const outcome = await watcher.settle(200);
		expect(outcome).toBe("none");
	});

	it("resolves 'completed' when a loading then complete transition occurs", async () => {
		const { deps, fire } = makeDeps();
		const watcher = watchForTriggeredNavigation(1, deps, 1000);
		const p = watcher.settle(500);
		fire(1, { status: "loading" }, { status: "loading" });
		fire(1, { status: "complete" }, { status: "complete" });
		expect(await p).toBe("completed");
	});

	it("rejects when a loading transition never completes", async () => {
		const { deps, fire } = makeDeps();
		// Short load timeout so the test doesn't hang on the 25s default.
		const watcher = watchForTriggeredNavigation(1, deps, 300);
		const p = watcher.settle(500);
		fire(1, { status: "loading" }, { status: "loading" });
		await expect(p).rejects.toThrow(/did not finish loading/i);
	});

	it("ignores update events for other tab IDs", async () => {
		const { deps, fire } = makeDeps();
		const watcher = watchForTriggeredNavigation(1, deps, 1000);
		const p = watcher.settle(200);
		fire(2, { status: "loading" }, { status: "loading" });
		fire(2, { status: "complete" }, { status: "complete" });
		expect(await p).toBe("none");
	});

	it("cancel() removes the listener", () => {
		const { deps, listeners } = makeDeps();
		const watcher = watchForTriggeredNavigation(1, deps, 1000);
		expect(listeners.length).toBe(1);
		watcher.cancel();
		expect(listeners.length).toBe(0);
	});

	it("ignores a loading event whose URL matches the baseline (unrelated reload)", async () => {
		const { deps, fire } = makeDeps();
		const watcher = watchForTriggeredNavigation(1, deps, 1000);
		watcher.setBaseline("https://example.com/page");
		const p = watcher.settle(200);
		// Background navigation (ad refresh, polling reload) on the same URL.
		fire(
			1,
			{ status: "loading", url: "https://example.com/page" },
			{ status: "loading", url: "https://example.com/page" },
		);
		expect(await p).toBe("none");
	});

	it("counts a loading event whose URL differs from the baseline", async () => {
		const { deps, fire } = makeDeps();
		const watcher = watchForTriggeredNavigation(1, deps, 1000);
		watcher.setBaseline("https://example.com/page");
		const p = watcher.settle(500);
		// Real navigation triggered by the click — URL differs from baseline.
		fire(
			1,
			{ status: "loading", url: "https://example.com/other" },
			{ status: "loading", url: "https://example.com/other" },
		);
		fire(
			1,
			{ status: "complete", url: "https://example.com/other" },
			{ status: "complete", url: "https://example.com/other" },
		);
		expect(await p).toBe("completed");
	});

	it("ignores a loading event with no URL when a baseline is set", async () => {
		// changeInfo from chrome.tabs.onUpdated may omit the URL on the very
		// first loading event for a tab. We treat "no URL" conservatively as
		// "could be a reload" — only count it if the baseline is unset.
		const { deps, fire } = makeDeps();
		const watcher = watchForTriggeredNavigation(1, deps, 1000);
		watcher.setBaseline("https://example.com/page");
		const p = watcher.settle(200);
		fire(1, { status: "loading" }, { status: "loading" });
		expect(await p).toBe("none");
	});
});

describe("waitForTabComplete", () => {
	it("resolves on status complete", async () => {
		const { deps, fire } = makeDeps();
		const p = new Promise<chrome.tabs.Tab>((resolve) => {
			waitForTabComplete(1, deps, resolve, () => {}, 1000);
		});
		fire(1, { status: "complete" }, { status: "complete" });
		expect((await p).status).toBe("complete");
	});

	it("resolves on a same-document URL change with complete status", async () => {
		const { deps, fire } = makeDeps();
		const p = new Promise<chrome.tabs.Tab>((resolve) => {
			waitForTabComplete(1, deps, resolve, () => {}, 1000);
		});
		fire(1, { url: "https://x/#hash" }, { status: "complete" });
		expect((await p).status).toBe("complete");
	});

	it("rejects on timeout", async () => {
		const { deps } = makeDeps();
		await expect(
			new Promise<chrome.tabs.Tab>((_resolve, reject) => {
				waitForTabComplete(1, deps, _resolve, reject, 200);
			}),
		).rejects.toThrow(/did not finish loading/i);
	});
});
