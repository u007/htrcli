/**
 * Session-recording commands driven remotely over native messaging (htrcli).
 *
 * This is the extension's OWN recorder — interaction steps plus screenshots,
 * persisted in IndexedDB. It is deliberately NOT the CDP screencast video
 * recorder that `htrcli record` uses: that one needs `Page.startScreencast`
 * and only works on Chrome. Everything here is browser-agnostic (it relies on
 * `chrome.runtime.sendMessage` and IndexedDB only), so the same commands work
 * on Chrome and on Firefox.
 *
 * The handlers are pure with respect to browser state: every side effect goes
 * through the injected {@link RecordingCommandDeps}. That keeps the module
 * unit-testable without a browser (see recordingCommands.test.ts) and lets
 * `background/index.ts` hand over the real recorder functions.
 */

import type { BrowserType, Command } from "../types/commands";
import type {
	Annotation,
	RecordingSession,
	RecordingStep,
	SessionMetadata,
} from "../types/recording";
import { getBrowserType } from "./browserType";

/** Every command action this module handles. */
export const RECORDING_ACTIONS: readonly string[] = [
	"recordingStart",
	"recordingStop",
	"recordingStatus",
	"recordingList",
	"recordingGet",
	"recordingDelete",
];

const RECORDING_ACTION_SET = new Set(RECORDING_ACTIONS);

/** True when the action is one of the session-recording commands. */
export function isRecordingAction(action: string): boolean {
	return RECORDING_ACTION_SET.has(action);
}

/**
 * Side effects required by the recording commands. Supplied by the background
 * service worker, which owns the live session and the IndexedDB handles.
 */
export interface RecordingCommandDeps {
	/** The in-flight session, or null when nothing is recording. */
	getActiveSession: () => RecordingSession | null;
	/** Begin a new session recording the active tab from now on. */
	startRecording: (
		title: string,
		hasAudio: boolean,
	) => Promise<RecordingSession>;
	/** Finish the active session and persist it. */
	stopRecording: () => Promise<RecordingSession | null>;
	/** All persisted sessions, any order. */
	listSessions: () => Promise<SessionMetadata[]>;
	/** Load one persisted session including steps/annotations. */
	loadSession: (sessionId: string) => Promise<RecordingSession | null>;
	/** Remove one persisted session. Resolves false when it did not exist. */
	deleteSession: (sessionId: string) => Promise<boolean>;
}

/** Default page size for `recordingList`. */
export const DEFAULT_LIST_LIMIT = 50;
/** Hard ceiling for `recordingList`, so a caller cannot request everything. */
export const MAX_LIST_LIMIT = 500;

// ─── Shapes ──────────────────────────────────────────────────────────

/** `recordingList` result. */
export interface RecordingListData {
	sessions: SessionMetadata[];
	/** Total sessions stored, before limit/offset were applied. */
	total: number;
	/** Echo of the applied window, so a caller can page deterministically. */
	limit: number;
	offset: number;
	/**
	 * Which browser produced this list ("chrome" | "firefox"). The daemon
	 * selects a relay by an advisory --browser hint and silently falls back when
	 * the named profile is not connected, so this is the only way a caller can
	 * tell which profile actually answered. The extension is the authority on
	 * its own identity, so it reports itself rather than being labelled.
	 */
	browser: BrowserType;
}

/** `recordingGet` result. */
export interface RecordingGetData {
	session: RecordingSession;
	/**
	 * True when screenshot/audio base64 blobs were stripped to keep the payload
	 * inside the native-messaging message limit. Re-run with
	 * `includeMedia: true` to get them.
	 */
	mediaStripped: boolean;
}

/** `recordingStart` / `recordingStop` / `recordingStatus` results. */
export interface RecordingStateData {
	/** True while a session is actively recording. */
	recording: boolean;
	/** The active session's metadata, or the just-finished session on stop. */
	session: SessionMetadata | null;
}

/** `recordingDelete` result. */
export interface RecordingDeleteData {
	id: string;
	deleted: boolean;
}

// ─── Helpers ────────────────────────────────────────────────────────

/** Project a full session down to the lightweight metadata record. */
export function toSessionMetadata(session: RecordingSession): SessionMetadata {
	return {
		id: session.id,
		title: session.title,
		startTime: session.startTime,
		endTime: session.endTime,
		hasAudio: session.hasAudio,
		stepCount: session.steps.length,
		annotationCount: session.annotations.length,
	};
}

/**
 * Remove base64 screenshot/audio payloads from a session.
 *
 * Every step carries a full-page PNG data URL, so a session with a few dozen
 * steps runs into the tens of megabytes. Native messaging frames messages with
 * a 4-byte length prefix but a multi-megabyte reply is still slow and fragile,
 * so `recordingGet` strips the media by default and the caller opts back in.
 */
export function stripMediaData(session: RecordingSession): RecordingSession {
	return {
		...session,
		steps: session.steps.map((step: RecordingStep) => {
			const { screenshotData, audioData, ...rest } = step;
			void screenshotData;
			void audioData;
			return rest as RecordingStep;
		}),
		annotations: session.annotations.map((annotation: Annotation) => {
			const { screenshotData, audioData, ...rest } = annotation;
			void screenshotData;
			void audioData;
			return rest as Annotation;
		}),
	};
}

/** True when the session actually carries media blobs worth stripping. */
export function hasMediaData(session: RecordingSession): boolean {
	return (
		session.steps.some((s) => s.screenshotData || s.audioData) ||
		session.annotations.some((a) => a.screenshotData || a.audioData)
	);
}

function readString(options: Record<string, unknown> | undefined, key: string) {
	const value = options?.[key];
	return typeof value === "string" && value.length > 0 ? value : undefined;
}

function readBoolean(
	options: Record<string, unknown> | undefined,
	key: string,
): boolean | undefined {
	const value = options?.[key];
	return typeof value === "boolean" ? value : undefined;
}

/** Clamp a caller-supplied page size into `[1, MAX_LIST_LIMIT]`. */
function resolveLimit(raw: unknown): number {
	const n = typeof raw === "number" ? raw : Number(raw);
	if (!Number.isFinite(n)) return DEFAULT_LIST_LIMIT;
	return Math.min(Math.max(Math.trunc(n), 1), MAX_LIST_LIMIT);
}

/** Clamp a caller-supplied offset to a non-negative integer. */
function resolveOffset(raw: unknown): number {
	const n = typeof raw === "number" ? raw : Number(raw);
	if (!Number.isFinite(n) || n < 0) return 0;
	return Math.trunc(n);
}

/**
 * Sort newest-first and apply limit/offset.
 *
 * Sorting is explicit rather than inherited from the store: `getAllSessions`
 * orders by its own index and the ordering is not part of its contract, and a
 * listing that silently changes order between calls is impossible to page over.
 */
export function paginateSessions(
	sessions: SessionMetadata[],
	limit: number,
	offset: number,
): { page: SessionMetadata[]; total: number } {
	const sorted = [...sessions].sort((a, b) => {
		if (b.startTime !== a.startTime) return b.startTime - a.startTime;
		return a.id.localeCompare(b.id);
	});
	return {
		page: sorted.slice(offset, offset + limit),
		total: sorted.length,
	};
}

/**
 * Throw when `sessionId` names the session that is currently recording.
 *
 * The in-flight session is not in IndexedDB yet (persistence happens on stop),
 * and `stopRecording` rewrites the row with an unconditional put — so deleting
 * it would report success, make the session vanish from every list, and then
 * have it silently reappear on the next stop. That bug shipped once already.
 *
 * Exported so the `DELETE_SESSION` message the side panel uses goes through the
 * identical guard as the htrcli `recordingDelete` command, rather than a UI
 * path being free to bypass it.
 */
export function assertNotLiveSession(
	getActiveSession: () => RecordingSession | null,
	sessionId: string,
): void {
	const active = getActiveSession();
	if (active?.isRecording && active.id === sessionId) {
		throw new Error(
			`recording ${sessionId} is currently being recorded — stop it first, then delete`,
		);
	}
}

/** Default title when the caller does not supply one. */
export function defaultSessionTitle(now: number): string {
	const d = new Date(now);
	const pad = (n: number) => String(n).padStart(2, "0");
	return `Recording ${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// ─── Dispatch ───────────────────────────────────────────────────────

/**
 * Execute a session-recording command and return its `CommandResult.data`.
 *
 * Throws on invalid input or a missing session; the caller turns that into a
 * `command_result` with `success: false`.
 */
export async function handleRecordingCommand(
	deps: RecordingCommandDeps,
	command: Command,
): Promise<unknown> {
	const options = command.options;

	switch (command.action) {
		case "recordingStatus": {
			const active = deps.getActiveSession();
			return {
				recording: Boolean(active?.isRecording),
				session: active ? toSessionMetadata(active) : null,
			} satisfies RecordingStateData;
		}

		case "recordingStart": {
			// The "already recording" refusal deliberately lives in the real
			// startRecording (src/background/index.ts), not here. That function is
			// the single point where the in-memory session is mutated, and it is
			// reached from three entry points — this native command, the toolbar
			// popup, and the side panel. A guard in only one of them would let the
			// other two silently discard every step captured so far, because an
			// in-flight session is never written to IndexedDB until it stops.
			// So we just forward, and let the dep's rejection propagate.
			// Audio requires microphone capture. Default it OFF so a remote
			// caller can never turn on the mic without asking for it explicitly.
			const hasAudio = readBoolean(options, "hasAudio") ?? false;
			const title =
				readString(options, "title") ?? defaultSessionTitle(Date.now());
			const session = await deps.startRecording(title, hasAudio);
			return {
				recording: true,
				session: toSessionMetadata(session),
			} satisfies RecordingStateData;
		}

		case "recordingStop": {
			const session = await deps.stopRecording();
			if (!session) {
				throw new Error(
					"no recording in progress — start one with: htrcli recordings start",
				);
			}
			return {
				recording: false,
				session: toSessionMetadata(session),
			} satisfies RecordingStateData;
		}

		case "recordingList": {
			const limit = resolveLimit(options?.limit);
			const offset = resolveOffset(options?.offset);
			const sessions = await deps.listSessions();
			const { page, total } = paginateSessions(sessions, limit, offset);
			return {
				sessions: page,
				total,
				limit,
				offset,
				browser: getBrowserType(),
			} satisfies RecordingListData;
		}

		case "recordingGet": {
			const id = readString(options, "sessionId") ?? readString(options, "id");
			if (!id) {
				throw new Error("recordingGet requires options.sessionId");
			}
			const session = await deps.loadSession(id);
			if (!session) {
				throw new Error(`no recording found with id ${id}`);
			}
			const includeMedia = readBoolean(options, "includeMedia") ?? false;
			return {
				session: includeMedia ? session : stripMediaData(session),
				mediaStripped: !includeMedia && hasMediaData(session),
			} satisfies RecordingGetData;
		}

		case "recordingDelete": {
			const id = readString(options, "sessionId") ?? readString(options, "id");
			if (!id) {
				throw new Error("recordingDelete requires options.sessionId");
			}
			assertNotLiveSession(deps.getActiveSession, id);
			const deleted = await deps.deleteSession(id);
			return { id, deleted } satisfies RecordingDeleteData;
		}

		default:
			throw new Error(`not a recording action: ${command.action}`);
	}
}
