/**
 * Virtualized-list collection.
 *
 * Long lists (Gmail threads, Google Drive rows, Slack messages, Sheets) are
 * virtualized: the DOM only holds the rows currently in the viewport, so a
 * single `findAll` returns a truncated slice. This module scrolls the list's
 * scroll container progressively and accumulates every row it sees, de-duping
 * by a stable key so a row that survives a scroll is not counted twice.
 *
 * The scroll container is discovered rather than assumed:
 *   1. the nearest scrollable ancestor of the first match, when it can scroll;
 *   2. the nearest scrollable ancestor of the first match's *parent* (Gmail
 *      wraps rows in a table whose body scrolls);
 *   3. `document.scrollingElement` as the last resort.
 *
 * If the container cannot scroll, the current DOM rows are returned as-is —
 * the caller still gets a usable answer instead of an error.
 */

import type { RemoteElementInfo, TargetSelector } from "../types/commands";
import { findAllElements, getElementInfo, isRendered } from "./elementFinder";

export interface CollectOptions {
	/** Maximum scroll steps before giving up. */
	maxSteps?: number;
	/** Milliseconds to wait between scroll steps (lets the page render). */
	stepDelayMs?: number;
	/** Stop scrolling once this many elements have been collected. */
	maxItems?: number;
	/** Emit a ref for every collected element. */
	assignRef?: boolean;
	/**
	 * Drop rows that are not rendered. An SPA keeps its previous view in the DOM
	 * (Gmail is the extreme case: 87 `tr.zA` matches for 37 conversations), so a
	 * collection can accumulate rows the user cannot see.
	 *
	 * The check runs at HARVEST time, before a row is marked as seen: a row that
	 * is hidden on one pass and visible on a later one must still be collected
	 * once, and marking it seen first would silently drop it.
	 */
	visibleOnly?: boolean;
	/**
	 * Called for every newly collected row WHILE its node is still live and
	 * attached. Minting a ref after the scroll loop is unsound: a virtualized
	 * list has already detached or recycled the node by then. Because of this,
	 * the scroll position is NOT restored when onRow is set, so the rows stay
	 * where the collection left them.
	 */
	onRow?: (el: Element, info: RemoteElementInfo) => void;
}

export interface CollectResult {
	elements: RemoteElementInfo[];
	/**
	 * The live DOM nodes, parallel to `elements`. Kept out of the wire payload
	 * (it never crosses the message boundary) but needed to mint element refs
	 * for the collected rows.
	 */
	nodes: Element[];
	/** Number of scroll steps performed. */
	steps: number;
	/** True when collection stopped before the list was fully read. */
	truncated: boolean;
	/**
	 * Which limit stopped collection, or null when it was not truncated. Kept
	 * separate so a caller can tell an item cap from a step limit.
	 */
	truncatedBy: TruncationCause | null;
	/** True when a scrollable container was found and used. */
	scrolled: boolean;
	/**
	 * The weakest identity strategy that had to be used for any row.
	 * "index" means the rows could not be identified reliably, so the
	 * result may contain duplicates or be missing rows.
	 */
	keyStrategy: KeyStrategy;
}

const DEFAULT_MAX_STEPS = 200;
const DEFAULT_STEP_DELAY_MS = 120;

/** True when the element can scroll vertically right now. */
function isScrollable(el: Element): boolean {
	const anyEl = el as HTMLElement;
	if (typeof anyEl.scrollHeight !== "number") return false;
	if (anyEl.scrollHeight <= anyEl.clientHeight) return false;
	const style = window.getComputedStyle(anyEl);
	const overflowY = style.overflowY;
	return (
		overflowY === "auto" || overflowY === "scroll" || overflowY === "overlay"
	);
}

/**
 * How many ancestors to inspect when looking for the list's scroll container.
 * A table-wrapper layout (Gmail) puts the scroller several levels above the
 * row, so the walk has to be generous — but bounded.
 */
const MAX_ANCESTOR_WALK = 13;

/**
 * Find the element that actually scrolls the list: the closest scrollable
 * ancestor of the first match (which also covers the table-wrapper layout),
 * else the document's scrolling element.
 *
 * There used to be a second walk starting at the match's PARENT. It was
 * unreachable: every element it could reach except the 13th was already
 * visited by the first walk, so it could never change the result.
 */
export function findScrollContainer(
	first: Element | undefined,
): Element | null {
	if (first) {
		let node: Element | null = first;
		for (let i = 0; i < MAX_ANCESTOR_WALK && node; i++) {
			if (isScrollable(node)) return node;
			node = node.parentElement;
		}
	}
	const doc = document.scrollingElement ?? document.documentElement;
	if (doc && doc.scrollHeight > doc.clientHeight) return doc;
	return null;
}

/**
 * How a row was identified.
 *
 * - `key` — a stable per-row attribute (`data-id` / `data-item-id` /
 *   `data-msg-id`). Reliable: the same row keeps the same key across
 *   scrolls.
 * - `text` — the row's text content. Usable, but two genuinely different
 *   rows with identical text collapse into one.
 * - `index` — no id and no text, so the row is identified by its DOM node.
 *   A virtualized list that keeps a row's node while the row is on screen
 *   gives a correct count. A list that recycles nodes can make two different
 *   rows share one node, which drops a row, and one that re-creates a node
 *   for the same row counts it twice. Either way the caller is told through
 *   `keyStrategy`, so the result may be incomplete.
 */
export type KeyStrategy = "key" | "text" | "index";

/** The limit that stopped a collection early. */
export type TruncationCause = "maxItems" | "maxSteps";

/**
 * Build a stable identity for an element so the same row seen twice (before
 * and after a scroll) is only collected once. Returns `null` when the row has
 * no id and no text; the caller then falls back to node identity.
 */
function elementKey(
	el: Element,
): { key: string; strategy: KeyStrategy } | { key: null; strategy: "index" } {
	const anyEl = el as HTMLElement & {
		getAttribute(name: string): string | null;
	};
	const keyAttr =
		anyEl.getAttribute?.("data-item-id") ??
		anyEl.getAttribute?.("data-msg-id") ??
		anyEl.getAttribute?.("data-id") ??
		"";
	if (keyAttr) return { key: `k:${keyAttr}`, strategy: "key" };
	const text = (el.textContent ?? "").trim().slice(0, 120);
	if (text) return { key: `t:${text}`, strategy: "text" };
	return { key: null, strategy: "index" };
}

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Collect every element matching `target`, scrolling the list as needed.
 */
export async function collectAll(
	target: TargetSelector,
	options: CollectOptions = {},
): Promise<CollectResult> {
	const maxSteps = options.maxSteps ?? DEFAULT_MAX_STEPS;
	const stepDelayMs = options.stepDelayMs ?? DEFAULT_STEP_DELAY_MS;
	const maxItems = options.maxItems ?? 0;
	const onRow = options.onRow;
	const visibleOnly = options.visibleOnly === true;

	const seen = new Map<string, RemoteElementInfo>();
	const nodes: Element[] = [];
	// Node identity for rows that have no id and no text. The key is per call:
	// a node's position in the current query is NOT an identity, because every
	// harvest restarts at 0 and distinct rows would collide.
	const nodeIds = new WeakMap<Element, string>();
	let nodeCount = 0;
	const nodeKey = (el: Element): string => {
		const existing = nodeIds.get(el);
		if (existing !== undefined) return existing;
		nodeCount++;
		const created = `n:${nodeCount}`;
		nodeIds.set(el, created);
		return created;
	};
	let truncatedBy: TruncationCause | null = null;
	let steps = 0;
	let scrolled = false;
	let keyStrategy: KeyStrategy = "key";

	const harvest = (): number => {
		const elements = findAllElements(target);
		// The cap is enforced HERE, not only at the top of the scroll loop: a
		// list whose rows are already in the DOM is fully collected by the
		// first harvest, so a loop-level break would never bound the result.
		for (const el of elements) {
			// Hidden rows are skipped BEFORE the cap and before `seen`, so they
			// neither consume the item budget nor get locked out of a later pass
			// that renders them.
			if (visibleOnly && !isRendered(el)) continue;
			if (maxItems > 0 && seen.size >= maxItems) {
				truncatedBy = "maxItems";
				break;
			}
			const identity = elementKey(el);
			const key = identity.key === null ? nodeKey(el) : identity.key;
			// Track the weakest strategy used across all rows.
			if (identity.strategy === "index") {
				keyStrategy = "index";
			} else if (identity.strategy === "text" && keyStrategy === "key") {
				keyStrategy = "text";
			}
			if (seen.has(key)) continue;
			const info = getElementInfo(el);
			onRow?.(el, info);
			seen.set(key, info);
			nodes.push(el);
		}
		return elements.length;
	};

	harvest();

	const container = findScrollContainer(findAllElements(target)[0]);
	if (!container) {
		return {
			elements: [...seen.values()],
			nodes,
			steps: 0,
			truncated: false,
			truncatedBy: null,
			scrolled: false,
			keyStrategy,
		};
	}

	const anyContainer = container as HTMLElement;
	let previousScrollTop = anyContainer.scrollTop;
	let previousHeight = anyContainer.scrollHeight;
	let unchanged = 0;

	// Collecting scrolls the user's list to the bottom. Restore the original
	// position so a read-only collection does not visibly move the page. Skip
	// the restore when refs are minted: scrolling back recycles the rows those
	// refs point at, so they would resolve to the wrong rows afterwards.
	const originalScrollTop = previousScrollTop;
	try {
		for (; steps < maxSteps; steps++) {
			if (maxItems > 0 && seen.size >= maxItems) {
				truncatedBy = "maxItems";
				break;
			}
			const before = seen.size;
			anyContainer.scrollTop =
				anyContainer.scrollTop + anyContainer.clientHeight;
			await sleep(stepDelayMs);
			harvest();

			const currentScrollTop = anyContainer.scrollTop;
			const currentHeight = anyContainer.scrollHeight;
			scrolled = true;

			// No new rows AND the scroll position/height did not move: either the
			// end of the list was reached or the container refused to scroll.
			if (
				seen.size === before &&
				currentScrollTop === previousScrollTop &&
				currentHeight === previousHeight
			) {
				unchanged++;
				if (unchanged >= 2) break;
			} else {
				unchanged = 0;
			}
			previousScrollTop = currentScrollTop;
			previousHeight = currentHeight;
		}
	} finally {
		if (!onRow) anyContainer.scrollTop = originalScrollTop;
	}

	// Running out of steps only means truncation when the last step still made
	// progress: a step that changed nothing shows the list had already ended.
	// A step that did change something may have pending content, so it counts.
	if (truncatedBy === null && steps >= maxSteps && unchanged === 0) {
		truncatedBy = "maxSteps";
	}

	const elements = [...seen.values()];
	return {
		elements,
		nodes,
		steps,
		truncated: truncatedBy !== null,
		truncatedBy,
		scrolled,
		keyStrategy,
	};
}
