/**
 * Toolbar-icon wiring for the Firefox build.
 *
 * Kept out of `firefox-shims.ts` (which side-effect-imports
 * `webextension-polyfill` and therefore cannot be loaded in a plain unit test)
 * so the registration contract is testable. The shim — imported only by the
 * background entry — calls `registerSidebarToggle(browserApi)` at import time.
 *
 * A background-only registration matters: every extension context that adds an
 * `action.onClicked` listener is a candidate to run the toggle, so this must
 * not be imported from sidebar/options entries.
 */

export interface SidebarToggleApi {
	action?: {
		onClicked?: {
			addListener: (listener: () => void) => void;
		};
	};
	sidebarAction?: {
		toggle: () => Promise<void>;
	};
}

/**
 * Registers the toolbar click that toggles the Firefox sidebar.
 *
 * Returns true when the listener was installed. Missing APIs are logged rather
 * than thrown: a browser without the sidebar action must still serve recording
 * and remote control, so it should not be bricked by a UI-only capability gap.
 */
// Event objects that already carry the toggle listener. A second registration on
// the same event would stack a second toggle (two clicks = open then close).
const registeredEvents = new WeakSet<object>();

export function registerSidebarToggle(api: SidebarToggleApi): boolean {
	const onClicked = api.action?.onClicked;
	const sidebarAction = api.sidebarAction;

	if (!onClicked) {
		console.error(
			"[HTR NControl] action.onClicked is unavailable; the toolbar icon cannot toggle the sidebar.",
		);
		return false;
	}
	if (!sidebarAction?.toggle) {
		console.error(
			"[HTR NControl] sidebarAction.toggle is unavailable; the toolbar icon cannot toggle the sidebar.",
		);
		return false;
	}
	if (registeredEvents.has(onClicked)) {
		return true;
	}

	onClicked.addListener(() => {
		// `toggle()` is only allowed inside a user-action handler;
		// `action.onClicked` qualifies. Call it synchronously (no await before
		// it) so the gesture is still live. It is called as a method so `this`
		// is the sidebarAction object, as the browser expects.
		sidebarAction.toggle().catch((error: unknown) => {
			console.warn("[HTR NControl] sidebar toggle failed:", error);
		});
	});
	registeredEvents.add(onClicked);
	return true;
}
