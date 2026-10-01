/**
 * Toolbar popup: manual session-recording control.
 *
 * The popup is a thin client over the background service worker — it never
 * touches IndexedDB or the recorder directly. It sends the same
 * START_RECORDING / STOP_RECORDING messages the side panel uses, so a
 * recording started here is indistinguishable from one started remotely via
 * `htrcli recordings start` (same session store, same side-panel sync).
 */

import { useCallback, useEffect, useRef, useState } from "react";

import "./Popup.css";
import { getAllSessions } from "../db/index";
import type {
	DeleteSessionResultMessage,
	RecordingSession,
	RecordingStateMessage,
	SessionMetadata,
} from "../types/recording";

type Status =
	| { kind: "idle" }
	// GET_RECORDING_STATE hands back the full live session, not the lightweight
	// metadata record, so `session` is typed as RecordingSession here.
	| { kind: "ready"; recording: boolean; session: RecordingSession | null }
	| { kind: "busy" }
	| { kind: "error"; message: string };

export const Popup = () => {
	const [status, setStatus] = useState<Status>({ kind: "idle" });
	const [title, setTitle] = useState("");
	const [hasAudio, setHasAudio] = useState(false);
	const [recent, setRecent] = useState<SessionMetadata[]>([]);
	// Guards against a stale sendMessage response overwriting newer UI state
	// (the popup can be reopened/clicks land faster than a round trip).
	const requestSeq = useRef(0);

	const refresh = useCallback(() => {
		const seq = ++requestSeq.current;
		chrome.runtime
			.sendMessage({ type: "GET_RECORDING_STATE" })
			.then((response: RecordingStateMessage | undefined) => {
				if (seq !== requestSeq.current) return;
				setStatus({
					kind: "ready",
					recording: Boolean(response?.isRecording),
					session: response?.session ?? null,
				});
			})
			.catch((err: unknown) => {
				if (seq !== requestSeq.current) return;
				setStatus({
					kind: "error",
					message: err instanceof Error ? err.message : String(err),
				});
			});
	}, []);

	useEffect(refresh, [refresh]);

	// Derived flag rather than reading `status` inside the effect: it keeps the
	// dependency list honest (a boolean, not a narrowed discriminated union) and
	// makes the poll condition obvious at a glance.
	const isRecording = status.kind === "ready" && status.recording;

	// Poll while recording so the step counter and elapsed time stay live
	// without the popup needing a message subscription.
	useEffect(() => {
		if (!isRecording) return;
		const timer = setInterval(refresh, 1000);
		return () => clearInterval(timer);
	}, [isRecording, refresh]);

	const loadRecent = useCallback(() => {
		// Read straight from IndexedDB (same as the side panel) rather than adding
		// a new background message type for a list the popup only renders.
		getAllSessions()
			.then((sessions) => setRecent(sessions.slice(0, 5)))
			// Cosmetic: the recent list is a shortcut, not the feature.
			.catch(() => setRecent([]));
	}, []);

	useEffect(loadRecent, [loadRecent]);

	/**
	 * Delete a stored recording via the background, never IndexedDB directly.
	 *
	 * The background owns the in-flight session, so it is the only place that can
	 * refuse to delete the recording currently being recorded. Going through it
	 * puts the UI on the same `assertNotLiveSession` guard as the htrcli
	 * `recordingDelete` command instead of letting the UI bypass it.
	 */
	const deleteSession = useCallback(
		async (sessionId: string) => {
			const response = (await chrome.runtime.sendMessage({
				type: "DELETE_SESSION",
				sessionId,
			})) as DeleteSessionResultMessage | undefined;
			if (!response?.success) {
				setStatus({
					kind: "error",
					message: response?.error ?? "Delete failed",
				});
				return;
			}
			loadRecent();
		},
		[loadRecent],
	);

	// React to recording-state changes the background broadcasts, whatever
	// initiated them. Without this the popup is stale whenever the side panel or
	// htrcli starts/stops a recording, and it would keep showing an idle screen
	// with a live "Start recording" button — clicking that used to overwrite the
	// in-flight session. (startRecording itself now refuses, so this is UX, not
	// data safety.)
	useEffect(() => {
		const onMessage = (message: unknown) => {
			const type = (message as { type?: string } | undefined)?.type;
			if (type === "RECORDING_STARTED" || type === "RECORDING_STOPPED") {
				refresh();
				if (type === "RECORDING_STOPPED") loadRecent();
			}
		};
		chrome.runtime.onMessage.addListener(onMessage);
		return () => chrome.runtime.onMessage.removeListener(onMessage);
	}, [refresh, loadRecent]);

	// The background reports failures in the RESPONSE BODY
	// ({success:false, error}), not via chrome.runtime.lastError — so
	// resolving this promise is not the same as succeeding. Without the
	// success check a failed start rendered as a normal idle screen.
	const send = useCallback(
		async (message: Record<string, unknown>): Promise<unknown> => {
			setStatus({ kind: "busy" });
			const response = (await new Promise((resolve, reject) => {
				chrome.runtime.sendMessage(message, (res) => {
					// `lastError` is only readable inside the callback — it is
					// cleared as soon as this returns.
					const err = chrome.runtime.lastError;
					if (err) {
						reject(new Error(err.message));
						return;
					}
					resolve(res);
				});
			})) as { success?: boolean; error?: string } | undefined;
			if (response && response.success === false) {
				throw new Error(response.error || "The extension rejected the request");
			}
			return response;
		},
		[],
	);

	const start = useCallback(async () => {
		try {
			await send({ type: "START_RECORDING", title, hasAudio });
			refresh();
		} catch (err) {
			setStatus({
				kind: "error",
				message: err instanceof Error ? err.message : String(err),
			});
		}
	}, [send, title, hasAudio, refresh]);

	const stop = useCallback(async () => {
		try {
			await send({ type: "STOP_RECORDING" });
			loadRecent();
			refresh();
		} catch (err) {
			setStatus({
				kind: "error",
				message: err instanceof Error ? err.message : String(err),
			});
		}
	}, [send, refresh, loadRecent]);

	const openSidePanel = useCallback(() => {
		// The side panel/sidebar holds the full session view (steps,
		// annotations, export). The popup is only a start/stop control.
		//
		// Three transports, tried in order:
		//  1. Chrome 116+ `chrome.sidePanel.open`.
		//  2. Firefox `chrome.sidebarAction.open` (webextension-polyfill maps
		//     the `browser.*` namespace onto `chrome.*`, so the shim's
		//     `sidebarAction` is reachable here). Needed because declaring
		//     `action.default_popup` suppresses `action.onClicked`, which used
		//     to be what opened the sidebar on a toolbar click.
		//  3. A plain tab — the last resort, works everywhere.
		const openInTab = () => {
			void chrome.tabs.create({ url: chrome.runtime.getURL("sidepanel.html") });
		};
		const sidePanel = chrome.sidePanel as typeof chrome.sidePanel & {
			open?: (opts: { windowId?: number; tabId?: number }) => Promise<void>;
		};
		if (typeof sidePanel?.open === "function") {
			void sidePanel
				.open({ windowId: chrome.windows.WINDOW_ID_CURRENT })
				.catch(() => {
					window.close();
					openInTab();
				});
			return;
		}
		const sidebarAction = (
			chrome as unknown as {
				sidebarAction?: { open?: () => Promise<void> };
			}
		).sidebarAction;
		if (typeof sidebarAction?.open === "function") {
			void sidebarAction.open().catch(() => {
				window.close();
				openInTab();
			});
			return;
		}
		openInTab();
	}, []);

	if (status.kind === "idle" || status.kind === "busy") {
		return (
			<main>
				<p className="muted">Checking recording state…</p>
			</main>
		);
	}

	if (status.kind === "error") {
		return (
			<main>
				<h3>HTR NControl</h3>
				<p className="error" role="alert">
					{status.message}
				</p>
				<button type="button" onClick={refresh}>
					Retry
				</button>
			</main>
		);
	}

	const { recording, session } = status;
	const elapsed = session?.startTime
		? Math.max(0, Math.round((Date.now() - session.startTime) / 1000))
		: 0;
	const mm = String(Math.floor(elapsed / 60)).padStart(2, "0");
	const ss = String(elapsed % 60).padStart(2, "0");

	return (
		<main>
			<h3>HTR NControl</h3>

			<p className={recording ? "status recording" : "status"}>
				{recording ? (
					<>
						<span className="dot" aria-hidden="true" />
						Recording {mm}:{ss}
					</>
				) : (
					"Not recording"
				)}
			</p>

			{recording ? (
				<>
					<p className="muted">
						{session ? `"${session.title}" · ` : ""}
						{session?.steps.length ?? 0} steps
					</p>
					<button type="button" className="danger" onClick={stop}>
						Stop recording
					</button>
				</>
			) : (
				<>
					<label className="field">
						<span>Title (optional)</span>
						<input
							type="text"
							value={title}
							placeholder="e.g. Checkout flow"
							maxLength={120}
							onChange={(e) => setTitle(e.target.value)}
						/>
					</label>
					<label className="field checkbox">
						<input
							type="checkbox"
							checked={hasAudio}
							onChange={(e) => setHasAudio(e.target.checked)}
						/>
						<span>Record audio</span>
					</label>
					<button type="button" className="primary" onClick={start}>
						Start recording
					</button>
				</>
			)}

			<button type="button" className="link" onClick={openSidePanel}>
				Open side panel
			</button>

			{recent.length > 0 && (
				<section aria-label="Recent recordings">
					<h4>Recent</h4>
					<ul className="recent">
						{recent.map((s) => (
							<li key={s.id}>
								<span className="recent-title">{s.title}</span>
								<span className="muted">
									{new Date(s.startTime).toLocaleString()} · {s.stepCount} steps
								</span>
								<button
									type="button"
									className="danger"
									aria-label={`Delete recording ${s.title}`}
									// Disabled while this session is live: the background would
									// refuse anyway, and a dead button with the reason in the
									// title beats an error the user has to dismiss.
									disabled={Boolean(session && session.id === s.id)}
									title={
										session && session.id === s.id
											? "This recording is in progress — stop it first"
											: "Delete this recording"
									}
									onClick={() => void deleteSession(s.id)}
								>
									Delete
								</button>
							</li>
						))}
					</ul>
				</section>
			)}

			<p className="muted small">
				Remote control:{" "}
				<code>htrcli recordings start|stop|list|get|export</code>
			</p>
		</main>
	);
};

export default Popup;
