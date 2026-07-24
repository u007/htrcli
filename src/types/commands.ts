/**
 * Command types for remote control of browser tabs
 * Used by the external API server to send commands to the extension
 */

// ─── Target Selectors ───────────────────────────────────────────────

/**
 * How to match text content
 */
export type TextMatchMode =
	| "exact"
	| "contains"
	| "regex"
	| "startsWith"
	| "endsWith";

/**
 * Selector for finding elements on the page
 * Multiple strategies can be combined; they are tried in priority order:
 *   selector > xpath > id > name > role > label > placeholder > text+tag
 */
export interface TargetSelector {
	/** CSS selector (highest priority) */
	selector?: string;
	/** XPath expression */
	xpath?: string;
	/** Element ID attribute */
	id?: string;
	/** Element name attribute */
	name?: string;
	/** ARIA role attribute */
	role?: string;
	/** Label text (finds associated form control) */
	label?: string;
	/** Placeholder attribute */
	placeholder?: string;
	/** Text content to match */
	text?: string;
	/** How to match text (default: "contains") */
	textMatch?: TextMatchMode;
	/** Case-sensitive text matching (default: false) */
	caseSensitive?: boolean;
	/** Tag name filter (used with text to narrow search) */
	tag?: string;
	/** Input type attribute (e.g. "text", "email", "checkbox") */
	type?: string;

	// ─── Multi-match handling ──────────────────────────────────────
	/** Which match to return when multiple elements match (0-based, default: 0) */
	index?: number;
	/** Return all matching elements instead of just the first */
	all?: boolean;

	// ─── Filters ───────────────────────────────────────────────────
	/** Only return visible elements */
	visible?: boolean;
	/** Only return enabled (non-disabled) elements */
	enabled?: boolean;

	// ─── Wait options ──────────────────────────────────────────────
	/** Wait for element to appear in DOM (default: false) */
	waitForAppear?: boolean;
	/** Timeout in ms for wait (default: 5000) */
	timeout?: number;
}

// ─── Element Info (returned by find/inspect commands) ───────────────

export interface RemoteElementInfo {
	tag: string;
	text: string;
	selector: string;
	xpath: string;
	type?: string;
	name?: string;
	id?: string;
	className?: string;
	ariaLabel?: string;
	value?: string;
	visible?: boolean;
	enabled?: boolean;
	boundingBox?: BoundingBox;
	attributes?: Record<string, string>;
}

export interface BoundingBox {
	x: number;
	y: number;
	width: number;
	height: number;
	top: number;
	bottom: number;
	left: number;
	right: number;
}

// ─── Command Actions ────────────────────────────────────────────────

export type CommandAction =
	// ─── Finding / Inspection ───────────────────────────────────────
	| "find"
	| "findAll"
	| "wait"
	| "isVisible"
	| "isEnabled"
	| "getValue"
	| "getAttribute"
	| "getText"
	| "getHTML"
	| "getOuterHTML"
	| "getBoundingBox"
	| "getComputedStyle"
	| "getPageInfo"
	| "xpath"
	// ─── Interaction ────────────────────────────────────────────────
	| "click"
	| "dblclick"
	| "rightclick"
	| "hover"
	| "focus"
	| "blur"
	| "scrollTo"
	| "fill"
	| "type"
	| "clear"
	| "select"
	| "check"
	| "uncheck"
	| "pressKey"
	| "selectText"
	// Internal: background asks the content script to prepare an element before a
	// trusted (CDP) click/key/type dispatch — wait actionable, scroll into view,
	// focus, and report coords/focus state. Not exposed to users.
	| "prepareClick"
	| "prepareKeys"
	// ─── Navigation ─────────────────────────────────────────────────
	| "navigate"
	| "reload"
	| "goBack"
	| "goForward"
	// ─── Screenshot ─────────────────────────────────────────────────
	| "screenshot"
	// ─── Script Execution ───────────────────────────────────────────
	| "evaluate"
	| "fetch"
	| "printToPDF"
	// ─── Highlight ──────────────────────────────────────────────────
	| "highlight"
	| "unhighlight"
	// ─── Tab Management ────────────────────────────────────────────
	| "listTabs"
	| "getTabInfo"
	// Background-handled: tabs with a live content script (diagnostics)
	| "getReadyTabs"
	| "switchTab"
	| "getSessionStorage"
	| "getLocalStorage"
	| "fetchInPage"
	| "fetchViaDOM"
	| "fetchFromCS"
	| "debuggerEval"
	| "openTab"
	| "closeTab"
	| "cdpNavigate"
	/** Internal: server uses this to route `evaluate` through CDP. Content
	 *  script forwards to background, which calls Runtime.evaluate. Keeps the
	 *  user's `evaluate` action surface intact. */
	| "evaluateViaCdp"
	// ─── Network Capture (background-handled via CDP / webRequest) ──
	| "networkCapture";

// ─── Command ────────────────────────────────────────────────────────

export interface Command {
	/** Unique command ID (for matching requests to responses) */
	id: string;
	/** The action to perform */
	action: CommandAction;
	/** Element target (required for most actions) */
	target?: TargetSelector;
	/** Value to use (for fill, type, pressKey, select, etc.) */
	value?: string;
	/** Additional options specific to the action */
	options?: Record<string, unknown>;
}

// ─── Command Result ─────────────────────────────────────────────────

export interface CommandResult {
	/** Matches the command ID */
	id: string;
	/** Whether the command succeeded */
	success: boolean;
	/** Result data (varies by action) */
	data?: unknown;
	/** Error message if failed */
	error?: string;
	/** Screenshot captured after command execution (base64 PNG) */
	screenshot?: string;
	/** Execution duration in ms */
	duration?: number;
	/** Page info after command */
	pageInfo?: PageInfo;
}

// ─── Page Info ──────────────────────────────────────────────────────

export interface PageInfo {
	url: string;
	title: string;
	domain: string;
	/** document.readyState — used by the server to detect load completion */
	readyState?: string;
	scrollX: number;
	scrollY: number;
	viewportWidth: number;
	viewportHeight: number;
	documentHeight: number;
	documentWidth: number;
	/** window.history.length — used by the background to detect
	 *  goBack/goForward availability. -1 means the page didn't report
	 *  it (e.g. content script didn't load in time). */
	historyLength?: number;
}

// ─── Tab Info ───────────────────────────────────────────────────────

export interface TabInfo {
	id: number;
	url: string;
	title: string;
	active: boolean;
	favIconUrl?: string;
}

// ─── Server Protocol Messages ───────────────────────────────────────

/**
 * Message sent from extension → server via WebSocket
 */
export interface ExtensionMessage {
	type: "register" | "command_result" | "heartbeat" | "error";
	tabId?: number;
	tabInfo?: TabInfo;
	commandId?: string;
	result?: CommandResult;
	error?: string;
	timestamp: number;
}

/**
 * Message sent from server → extension via WebSocket
 */
export interface ServerMessage {
	type: "command" | "ping" | "disconnect";
	tabId?: number;
	command?: Command;
	timestamp: number;
}

// ─── Server API Types ───────────────────────────────────────────────

export interface ApiResponse<T = unknown> {
	ok: boolean;
	data?: T;
	error?: string;
}

export interface CommandRequest {
	command: Command;
	/** Capture screenshot after command (default: false) */
	screenshot?: boolean;
	/** Timeout override in ms */
	timeout?: number;
}
