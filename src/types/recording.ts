// Element information captured during interactions
import type { Command } from "./commands";
export interface ElementInfo {
	tag: string;
	text: string; // Button/link text content
	selector: string; // Unique CSS selector
	type?: string; // For inputs: text, password, email, etc.
	name?: string; // Field name attribute
	id?: string; // Element ID if present
	className?: string; // Element class names
	ariaLabel?: string; // Accessibility label
}

// Individual recording step
export interface RecordingStep {
	id: string;
	timestamp: number; // ms from recording start
	type: "click" | "input" | "navigation";
	tabId: number;
	tabTitle: string;
	url: string;
	screenshotData?: string; // base64 (internal storage)
	audioData?: string; // base64 webm (internal storage)
	element?: ElementInfo;
	inputValue?: string; // Masked if sensitive
	isSensitive?: boolean;
}

// User annotation between steps
export interface Annotation {
	id: string;
	timestamp: number; // Position in timeline (between steps)
	text: string;
	screenshotData?: string; // Optional screenshot
	audioData?: string; // Optional audio note
}

// Recording session metadata stored in chrome.storage.local
export interface SessionMetadata {
	id: string;
	title: string;
	startTime: number;
	endTime?: number;
	hasAudio: boolean;
	stepCount: number;
	annotationCount: number;
}

// Full recording session
export interface RecordingSession {
	id: string;
	title: string;
	startTime: number;
	endTime?: number;
	isRecording: boolean;
	hasAudio: boolean;
	steps: RecordingStep[];
	annotations: Annotation[];
	trackedTabIds: number[];
}

// Connection mode tracked by the background and surfaced to the UI.
// - "native": connected via native messaging (htrcli relay → daemon)
// - "disconnected": transient — relay exited (daemon down), retrying with backoff
// - "unavailable": permanent — host not installed/forbidden, or max retries exceeded
export type ConnectionMode = "native" | "disconnected" | "unavailable";

// Console capture levels forwarded from the main-world wrapper.
export type ConsoleLevel = "log" | "warn" | "error" | "info" | "debug";

// Structured console payload captured in the page's MAIN world.
export interface ConsoleEntry {
	level: ConsoleLevel;
	args: string[];
	source?: string;
}

// Structured network payload captured via CDP (Chrome) or webRequest (Firefox).
export interface NetworkEntry {
	requestId: string;
	url: string;
	method: string;
	status?: number;
	requestHeaders?: Record<string, string>;
	responseHeaders?: Record<string, string>;
	bodyTruncated?: boolean; // true if the response body was capped
	body?: string; // response body, omitted on Firefox (webRequest can't cheaply read it)
	durationMs?: number;
}

// Message types for communication between components
export type MessageType =
	| "START_RECORDING"
	| "STOP_RECORDING"
	| "RECORDING_STARTED"
	| "RECORDING_STOPPED"
	| "CLICK_EVENT"
	| "INPUT_EVENT"
	| "NAVIGATION_EVENT"
	| "NEW_STEP"
	| "ADD_ANNOTATION"
	| "UPDATE_ANNOTATION"
	| "DELETE_ANNOTATION"
	| "GET_RECORDING_STATE"
	| "RECORDING_STATE"
	| "CAPTURE_SCREENSHOT"
	| "SCREENSHOT_CAPTURED"
	| "ENABLE_RECORDING"
	| "DISABLE_RECORDING"
	| "CONTENT_SCRIPT_READY"
	| "HIGHLIGHT_ELEMENT"
	| "HIDE_HIGHLIGHT"
	| "CONNECTION_STATUS"
	| "RECONNECT_NATIVE"
	| "CDP_EVAL"
	| "CONSOLE_ENTRY"
	// Server/WS path relays trusted (CDP) click/pressKey/type to the background,
	// which owns the debugger connection. The content script sends this and
	// awaits the CommandResult the background produces.
	| "CDP_INPUT";

// Base message structure
export interface BaseMessage {
	type: MessageType;
}

// Start recording message
export interface StartRecordingMessage extends BaseMessage {
	type: "START_RECORDING";
	title: string;
	hasAudio: boolean;
}

// Stop recording message
export interface StopRecordingMessage extends BaseMessage {
	type: "STOP_RECORDING";
}

// Recording started response
export interface RecordingStartedMessage extends BaseMessage {
	type: "RECORDING_STARTED";
	sessionId: string;
}

// Recording stopped response
export interface RecordingStoppedMessage extends BaseMessage {
	type: "RECORDING_STOPPED";
	sessionId: string;
}

// Click event from content script
export interface ClickEventMessage extends BaseMessage {
	type: "CLICK_EVENT";
	element: ElementInfo;
	url: string;
	timestamp: number;
}

// Input event from content script
export interface InputEventMessage extends BaseMessage {
	type: "INPUT_EVENT";
	element: ElementInfo;
	value: string;
	isSensitive: boolean;
	url: string;
	timestamp: number;
}

// Navigation event
export interface NavigationEventMessage extends BaseMessage {
	type: "NAVIGATION_EVENT";
	url: string;
	title: string;
	tabId: number;
	timestamp: number;
}

// New step notification to side panel
export interface NewStepMessage extends BaseMessage {
	type: "NEW_STEP";
	step: RecordingStep;
}

// Add annotation message
export interface AddAnnotationMessage extends BaseMessage {
	type: "ADD_ANNOTATION";
	text: string;
	timestamp: number;
	screenshotData?: string;
	audioData?: string;
}

// Update annotation message
export interface UpdateAnnotationMessage extends BaseMessage {
	type: "UPDATE_ANNOTATION";
	annotationId: string;
	text: string;
}

// Delete annotation message
export interface DeleteAnnotationMessage extends BaseMessage {
	type: "DELETE_ANNOTATION";
	annotationId: string;
}

// Get recording state message
export interface GetRecordingStateMessage extends BaseMessage {
	type: "GET_RECORDING_STATE";
}

// Recording state response
export interface RecordingStateMessage extends BaseMessage {
	type: "RECORDING_STATE";
	isRecording: boolean;
	session?: RecordingSession;
}

// Enable recording in content script
export interface EnableRecordingMessage extends BaseMessage {
	type: "ENABLE_RECORDING";
}

// Disable recording in content script
export interface DisableRecordingMessage extends BaseMessage {
	type: "DISABLE_RECORDING";
}

// Content script ready notification
export interface ContentScriptReadyMessage extends BaseMessage {
	type: "CONTENT_SCRIPT_READY";
	url: string;
}

// Highlight element message (for screenshot capture)
export interface HighlightElementMessage extends BaseMessage {
	type: "HIGHLIGHT_ELEMENT";
	selector: string;
}

// Hide highlight message
export interface HideHighlightMessage extends BaseMessage {
	type: "HIDE_HIGHLIGHT";
}

// Connection status message (background → sidepanel)
export interface ConnectionStatusMessage extends BaseMessage {
	type: "CONNECTION_STATUS";
	mode: ConnectionMode;
}

// MAIN-world console entry forwarded to the background for durable buffering.
export interface ConsoleEntryMessage extends BaseMessage {
	type: "CONSOLE_ENTRY";
	entry: ConsoleEntry;
}

// Content script → background: relay a trusted (CDP) input command (click /
// pressKey / type) to the background, which owns the debugger connection.
export interface CdpInputMessage extends BaseMessage {
	type: "CDP_INPUT";
	command: Command;
}

// Union type for all messages
export type RecordingMessage =
	| StartRecordingMessage
	| StopRecordingMessage
	| RecordingStartedMessage
	| RecordingStoppedMessage
	| ClickEventMessage
	| InputEventMessage
	| NavigationEventMessage
	| NewStepMessage
	| AddAnnotationMessage
	| UpdateAnnotationMessage
	| DeleteAnnotationMessage
	| GetRecordingStateMessage
	| RecordingStateMessage
	| EnableRecordingMessage
	| DisableRecordingMessage
	| ContentScriptReadyMessage
	| HighlightElementMessage
	| HideHighlightMessage
	| ConnectionStatusMessage
	| CdpInputMessage
	| ConsoleEntryMessage;

// Export format types
export interface ExportedStep {
	id: string;
	timestamp: number;
	type: "click" | "input" | "navigation";
	url: string;
	tabTitle: string;
	screenshotPath?: string;
	audioPath?: string;
	element?: ElementInfo;
	inputValue?: string;
	isSensitive?: boolean;
}

export interface ExportedAnnotation {
	id: string;
	timestamp: number;
	text: string;
	screenshotPath?: string;
	audioPath?: string;
}

export interface ExportedRecording {
	id: string;
	title: string;
	startTime: number;
	endTime?: number;
	hasAudio: boolean;
	steps: ExportedStep[];
	annotations: ExportedAnnotation[];
}

// Timeline item for unified rendering (step or annotation)
export type TimelineItem =
	| { type: "step"; data: RecordingStep }
	| { type: "annotation"; data: Annotation };

// Utility function to generate unique IDs
export function generateId(prefix: string = ""): string {
	return `${prefix}${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
}
