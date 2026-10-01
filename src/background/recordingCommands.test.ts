/**
 * Tests for the htrcli-driven session-recording commands.
 *
 * Everything runs against fake deps, so this proves the dispatch contract
 * (argument validation, error messages, ordering, pagination, media stripping)
 * without a browser. The wiring in nativeHost.ts / background/index.ts is
 * covered by the type checker and by manual verification.
 */

import { describe, expect, it } from "bun:test";
import type { Command } from "../types/commands";
import type { RecordingSession, SessionMetadata } from "../types/recording";
import { getBrowserType } from "./browserType";

// recordingList reports which browser answered, and getBrowserType() reads the
// `chrome` global. A real extension always has it; this harness does not, so
// provide a minimal one. `debugger` is present, so the detected value is
// "chrome" — the test asserts against getBrowserType() itself, so it stays
// correct either way.
(globalThis as { chrome?: unknown }).chrome ??= { debugger: {} };

import {
	assertNotLiveSession,
	DEFAULT_LIST_LIMIT,
	handleRecordingCommand,
	hasMediaData,
	isRecordingAction,
	MAX_LIST_LIMIT,
	paginateSessions,
	type RecordingCommandDeps,
	stripMediaData,
	toSessionMetadata,
} from "./recordingCommands";

// ─── Fixtures ──────────────────────────────────────────────────────

function meta(
	id: string,
	startTime: number,
	overrides: Partial<SessionMetadata> = {},
): SessionMetadata {
	return {
		id,
		title: `Session ${id}`,
		startTime,
		endTime: startTime + 1000,
		hasAudio: false,
		stepCount: 0,
		annotationCount: 0,
		...overrides,
	};
}

function session(
	id: string,
	overrides: Partial<RecordingSession> = {},
): RecordingSession {
	return {
		id,
		title: `Session ${id}`,
		startTime: 1000,
		endTime: 2000,
		isRecording: false,
		hasAudio: false,
		steps: [],
		annotations: [],
		trackedTabIds: [],
		...overrides,
	};
}

type Overrides = Partial<RecordingCommandDeps>;

function deps(overrides: Overrides = {}): RecordingCommandDeps {
	return {
		getActiveSession: () => null,
		startRecording: async (title, hasAudio) =>
			session("new", { title, hasAudio }),
		stopRecording: async () => null,
		listSessions: async () => [],
		loadSession: async () => null,
		deleteSession: async () => false,
		...overrides,
	};
}

function cmd(action: string, options?: Record<string, unknown>): Command {
	// `action` and `id` are widened to string here; the Command union is only
	// enforced at the call sites in nativeHost.ts, and this module dispatches on
	// the raw string so the tests can also cover a non-recording action.
	return { id: "cmd-1", action, options } as Command;
}

// ─── isRecordingAction ─────────────────────────────────────────────

describe("isRecordingAction", () => {
	it("recognises every documented recording action", () => {
		for (const action of [
			"recordingStart",
			"recordingStop",
			"recordingStatus",
			"recordingList",
			"recordingGet",
			"recordingDelete",
		]) {
			expect(isRecordingAction(action)).toBe(true);
		}
	});

	it("does not claim unrelated actions", () => {
		// `record` is the CDP video recorder's verb and must not be intercepted.
		for (const action of ["record", "click", "navigate", "dialogPolicy", ""]) {
			expect(isRecordingAction(action)).toBe(false);
		}
	});
});

// ─── recordingStatus ───────────────────────────────────────────────

describe("recordingStatus", () => {
	it("reports idle when nothing is recording", async () => {
		const data = await handleRecordingCommand(deps(), cmd("recordingStatus"));
		expect(data).toEqual({ recording: false, session: null });
	});

	it("reports the active session metadata while recording", async () => {
		const active = session("live", {
			isRecording: true,
			steps: [{ id: "s1" } as RecordingSession["steps"][0]],
			annotations: [{ id: "a1" } as RecordingSession["annotations"][0]],
		});
		const data = await handleRecordingCommand(
			deps({ getActiveSession: () => active }),
			cmd("recordingStatus"),
		);
		expect(data).toMatchObject({
			recording: true,
			session: { id: "live", stepCount: 1, annotationCount: 1 },
		});
	});

	it("reports recording:false for a paused (non-recording) active session", async () => {
		const data = await handleRecordingCommand(
			deps({ getActiveSession: () => session("paused") }),
			cmd("recordingStatus"),
		);
		expect(data).toMatchObject({ recording: false, session: { id: "paused" } });
	});
});

// ─── recordingStart ────────────────────────────────────────────────

describe("recordingStart", () => {
	it("passes an explicit title and audio flag through", async () => {
		const data = await handleRecordingCommand(
			deps(),
			cmd("recordingStart", { title: "Checkout flow", hasAudio: true }),
		);
		expect(data).toMatchObject({
			recording: true,
			session: { title: "Checkout flow", hasAudio: true },
		});
	});

	it("defaults audio OFF so a remote caller cannot silently open the mic", async () => {
		const seen: boolean[] = [];
		await handleRecordingCommand(
			deps({
				startRecording: async (_title, hasAudio) => {
					seen.push(hasAudio);
					return session("new");
				},
			}),
			cmd("recordingStart", { title: "t" }),
		);
		expect(seen).toEqual([false]);
	});

	it("generates a dated default title when none is supplied", async () => {
		const data = await handleRecordingCommand(deps(), cmd("recordingStart"));
		const title = (data as { session: { title: string } }).session.title;
		expect(title).toMatch(/^Recording \d{4}-\d{2}-\d{2} \d{2}:\d{2}$/);
	});

	// The in-flight guard lives in the real startRecording (background/index.ts),
	// not here — see the note in recordingCommands.ts. What this module owes is
	// that the dep's refusal reaches the caller instead of being swallowed into
	// a bogus success. Without this, `recordings start` on a live recorder would
	// report `recording: true` while the session was never created.
	it("propagates the dep's already-recording refusal to the caller", async () => {
		await expect(
			handleRecordingCommand(
				deps({
					getActiveSession: () => session("live", { isRecording: true }),
					startRecording: async () => {
						throw new Error(
							'a recording is already in progress (session_live, "Session live") — stop it first',
						);
					},
				}),
				cmd("recordingStart"),
			),
		).rejects.toThrow(/already in progress \(session_live/);
	});

	it("forwards the resolved title and audio to the dep", async () => {
		const seen: { title: string; hasAudio: boolean }[] = [];
		await handleRecordingCommand(
			deps({
				startRecording: async (title, hasAudio) => {
					seen.push({ title, hasAudio });
					return session("new");
				},
			}),
			cmd("recordingStart", { title: "Checkout flow", hasAudio: true }),
		);
		expect(seen).toEqual([{ title: "Checkout flow", hasAudio: true }]);
	});

	it("allows starting over a finished (non-recording) session", async () => {
		const data = await handleRecordingCommand(
			deps({ getActiveSession: () => session("old", { isRecording: false }) }),
			cmd("recordingStart"),
		);
		expect(data).toMatchObject({ recording: true });
	});

	it("ignores a non-boolean hasAudio instead of coercing it", async () => {
		const data = await handleRecordingCommand(
			deps(),
			cmd("recordingStart", { title: "t", hasAudio: "yes" }),
		);
		expect(data).toMatchObject({ session: { hasAudio: false } });
	});
});

// ─── recordingStop ─────────────────────────────────────────────────

describe("recordingStop", () => {
	it("returns the finished session", async () => {
		const data = await handleRecordingCommand(
			deps({ stopRecording: async () => session("done") }),
			cmd("recordingStop"),
		);
		expect(data).toMatchObject({
			recording: false,
			session: { id: "done" },
		});
	});

	it("errors when no recording is in progress", async () => {
		await expect(
			handleRecordingCommand(deps(), cmd("recordingStop")),
		).rejects.toThrow(/no recording in progress/);
	});
});

// ─── recordingList ─────────────────────────────────────────────────

describe("recordingList", () => {
	const many = [
		meta("old", 1000),
		meta("newest", 5000),
		meta("middle", 3000),
		meta("tie-b", 4000),
		meta("tie-a", 4000),
	];

	it("sorts newest-first, breaking ties by id", async () => {
		const data = await handleRecordingCommand(
			deps({ listSessions: async () => many }),
			cmd("recordingList"),
		);
		expect(
			(data as { sessions: SessionMetadata[] }).sessions.map((s) => s.id),
		).toEqual(["newest", "tie-a", "tie-b", "middle", "old"]);
	});

	// The list output is how a user learns WHICH browser answered, which is the
	// only way to tell that a --browser hint silently fell back. The extension is
	// the authority on its own identity, so it reports it rather than the daemon
	// guessing from the connection.
	it("reports which browser answered", async () => {
		const data = await handleRecordingCommand(
			deps({ listSessions: async () => many }),
			cmd("recordingList"),
		);
		expect(data).toMatchObject({ browser: getBrowserType() });
	});

	it("reports the pre-pagination total", async () => {
		const data = await handleRecordingCommand(
			deps({ listSessions: async () => many }),
			cmd("recordingList", { limit: 2 }),
		);
		expect(data).toMatchObject({ total: 5, limit: 2, offset: 0 });
		expect((data as { sessions: unknown[] }).sessions).toHaveLength(2);
	});

	it("pages with offset without re-sorting", async () => {
		const data = await handleRecordingCommand(
			deps({ listSessions: async () => many }),
			cmd("recordingList", { limit: 2, offset: 2 }),
		);
		expect(
			(data as { sessions: SessionMetadata[] }).sessions.map((s) => s.id),
		).toEqual(["tie-b", "middle"]);
	});

	it("returns an empty page past the end rather than failing", async () => {
		const data = await handleRecordingCommand(
			deps({ listSessions: async () => many }),
			cmd("recordingList", { offset: 99 }),
		);
		expect(data).toMatchObject({ total: 5 });
		expect((data as { sessions: unknown[] }).sessions).toEqual([]);
	});

	it("defaults the limit when none is given", async () => {
		const data = await handleRecordingCommand(
			deps({ listSessions: async () => many }),
			cmd("recordingList"),
		);
		expect(data).toMatchObject({ limit: DEFAULT_LIST_LIMIT, offset: 0 });
	});

	it("clamps a limit above the ceiling instead of returning everything", async () => {
		const data = await handleRecordingCommand(
			deps({ listSessions: async () => many }),
			cmd("recordingList", { limit: 100000 }),
		);
		expect(data).toMatchObject({ limit: MAX_LIST_LIMIT });
	});

	it("replaces a nonsensical limit with the default", async () => {
		const data = await handleRecordingCommand(
			deps({ listSessions: async () => many }),
			cmd("recordingList", { limit: "abc" }),
		);
		expect(data).toMatchObject({ limit: DEFAULT_LIST_LIMIT });
	});

	it("treats a negative limit as 1 rather than an empty list", async () => {
		const data = await handleRecordingCommand(
			deps({ listSessions: async () => many }),
			cmd("recordingList", { limit: -5 }),
		);
		expect(data).toMatchObject({ limit: 1 });
		expect((data as { sessions: unknown[] }).sessions).toHaveLength(1);
	});

	it("clamps a negative offset to 0", async () => {
		const data = await handleRecordingCommand(
			deps({ listSessions: async () => many }),
			cmd("recordingList", { offset: -3 }),
		);
		expect(data).toMatchObject({ offset: 0 });
	});
});

describe("paginateSessions", () => {
	it("does not mutate the caller's array", () => {
		const input = [meta("a", 1000), meta("b", 2000)];
		paginateSessions(input, 10, 0);
		expect(input.map((s) => s.id)).toEqual(["a", "b"]);
	});
});

// ─── recordingGet ──────────────────────────────────────────────────

describe("recordingGet", () => {
	const withMedia = session("s1", {
		steps: [
			{
				id: "st1",
				screenshotData: "data:image/png;base64,AAA",
			} as RecordingSession["steps"][0],
		],
		annotations: [
			{ id: "an1", audioData: "AAA" } as RecordingSession["annotations"][0],
		],
	});

	it("strips media by default and says so", async () => {
		const data = await handleRecordingCommand(
			deps({ loadSession: async () => withMedia }),
			cmd("recordingGet", { sessionId: "s1" }),
		);
		expect(data).toMatchObject({ mediaStripped: true });
		const got = (data as { session: RecordingSession }).session;
		expect(got.steps[0].screenshotData).toBeUndefined();
		expect(got.annotations[0].audioData).toBeUndefined();
		// Non-media fields survive so the call is still useful.
		expect(got.steps[0].id).toBe("st1");
		expect(got.id).toBe("s1");
	});

	it("keeps media when explicitly requested", async () => {
		const data = await handleRecordingCommand(
			deps({ loadSession: async () => withMedia }),
			cmd("recordingGet", { sessionId: "s1", includeMedia: true }),
		);
		expect(data).toMatchObject({ mediaStripped: false });
		const got = (data as { session: RecordingSession }).session;
		expect(got.steps[0].screenshotData).toBe("data:image/png;base64,AAA");
	});

	it("reports mediaStripped:false for a media-free session", async () => {
		const data = await handleRecordingCommand(
			deps({ loadSession: async () => session("s2") }),
			cmd("recordingGet", { sessionId: "s2" }),
		);
		expect(data).toMatchObject({ mediaStripped: false });
	});

	it("accepts `id` as an alias for `sessionId`", async () => {
		const data = await handleRecordingCommand(
			deps({ loadSession: async () => session("s1") }),
			cmd("recordingGet", { id: "s1" }),
		);
		expect(data).toMatchObject({ session: { id: "s1" } });
	});

	it("requires an id", async () => {
		await expect(
			handleRecordingCommand(deps(), cmd("recordingGet")),
		).rejects.toThrow(/requires options.sessionId/);
	});

	it("errors on an unknown id", async () => {
		await expect(
			handleRecordingCommand(
				deps({ loadSession: async () => null }),
				cmd("recordingGet", { sessionId: "nope" }),
			),
		).rejects.toThrow(/no recording found with id nope/);
	});

	it("does not mutate the loaded session when stripping", async () => {
		const data = await handleRecordingCommand(
			deps({ loadSession: async () => withMedia }),
			cmd("recordingGet", { sessionId: "s1" }),
		);
		// The store-backed object is reused across calls; stripping must copy.
		expect(withMedia.steps[0].screenshotData).toBe("data:image/png;base64,AAA");
		expect(
			(data as { session: RecordingSession }).session.steps[0].screenshotData,
		).toBeUndefined();
	});
});

// ─── recordingDelete ───────────────────────────────────────────────

describe("recordingDelete", () => {
	it("reports a successful delete", async () => {
		const data = await handleRecordingCommand(
			deps({ deleteSession: async () => true }),
			cmd("recordingDelete", { sessionId: "s1" }),
		);
		expect(data).toEqual({ id: "s1", deleted: true });
	});

	it("reports deleted:false for an unknown id rather than erroring", async () => {
		const data = await handleRecordingCommand(
			deps({ deleteSession: async () => false }),
			cmd("recordingDelete", { sessionId: "ghost" }),
		);
		expect(data).toEqual({ id: "ghost", deleted: false });
	});

	it("requires an id", async () => {
		await expect(
			handleRecordingCommand(deps(), cmd("recordingDelete")),
		).rejects.toThrow(/requires options.sessionId/);
	});

	// Regression: deleting the live session used to report success. It is not
	// in IndexedDB yet, and `stop` rewrites the row unconditionally — so the
	// session reappeared on the next stop.
	it("refuses to delete the in-flight session", async () => {
		let deleted = 0;
		await expect(
			handleRecordingCommand(
				deps({
					getActiveSession: () => session("live", { isRecording: true }),
					deleteSession: async () => {
						deleted++;
						return true;
					},
				}),
				cmd("recordingDelete", { sessionId: "live" }),
			),
		).rejects.toThrow(/currently being recorded/);
		expect(deleted).toBe(0);
	});

	it("still deletes a different session while one is recording", async () => {
		const data = await handleRecordingCommand(
			deps({
				getActiveSession: () => session("live", { isRecording: true }),
				deleteSession: async () => true,
			}),
			cmd("recordingDelete", { sessionId: "other" }),
		);
		expect(data).toEqual({ id: "other", deleted: true });
	});
});

// ─── Unknown action ────────────────────────────────────────────────

describe("handleRecordingCommand", () => {
	it("throws for a non-recording action rather than returning a bare null", async () => {
		// Guards the nativeHost dispatch branch: a mis-wired action must surface
		// as an error, never as a silent success with undefined data.
		await expect(handleRecordingCommand(deps(), cmd("record"))).rejects.toThrow(
			/not a recording action: record/,
		);
	});
});

// ─── Pure helpers ──────────────────────────────────────────────────

describe("toSessionMetadata", () => {
	it("counts steps and annotations from the session body", () => {
		const got = toSessionMetadata(
			session("s1", {
				steps: [{ id: "a" }, { id: "b" }] as RecordingSession["steps"],
				annotations: [{ id: "c" }] as RecordingSession["annotations"],
			}),
		);
		expect(got).toMatchObject({ id: "s1", stepCount: 2, annotationCount: 1 });
	});
});

describe("stripMediaData / hasMediaData", () => {
	it("detects media on a step and on an annotation", () => {
		expect(
			hasMediaData(
				session("s", {
					steps: [
						{ id: "a", screenshotData: "x" },
					] as RecordingSession["steps"],
				}),
			),
		).toBe(true);
		expect(
			hasMediaData(
				session("s", {
					annotations: [
						{ id: "a", audioData: "x" },
					] as RecordingSession["annotations"],
				}),
			),
		).toBe(true);
	});

	it("reports no media for a bare session", () => {
		expect(hasMediaData(session("s"))).toBe(false);
	});

	it("leaves trackedTabIds and isRecording intact", () => {
		const stripped = stripMediaData(
			session("s", { isRecording: true, trackedTabIds: [1, 2] }),
		);
		expect(stripped.isRecording).toBe(true);
		expect(stripped.trackedTabIds).toEqual([1, 2]);
	});
});

describe("assertNotLiveSession", () => {
	// The DELETE_SESSION message the popup sends and the recordingDelete command
	// both route through this one function, so these are the tests that make the
	// guard structural rather than a rule each call site has to remember.
	it("refuses the session that is currently recording", () => {
		expect(() =>
			assertNotLiveSession(
				() => session("live", { isRecording: true }),
				"live",
			),
		).toThrow(/currently being recorded/);
	});

	it("allows a different session while one is recording", () => {
		expect(() =>
			assertNotLiveSession(
				() => session("live", { isRecording: true }),
				"other",
			),
		).not.toThrow();
	});

	it("allows deleting a finished session", () => {
		expect(() =>
			assertNotLiveSession(
				() => session("done", { isRecording: false }),
				"done",
			),
		).not.toThrow();
	});

	it("allows deleting when nothing is recording", () => {
		expect(() => assertNotLiveSession(() => null, "anything")).not.toThrow();
	});

	it("names the session in the message so the UI can explain the refusal", () => {
		expect(() =>
			assertNotLiveSession(
				() => session("abc123", { isRecording: true }),
				"abc123",
			),
		).toThrow(/abc123/);
	});
});
