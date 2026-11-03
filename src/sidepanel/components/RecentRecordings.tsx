import { useCallback, useState } from "react";
import type {
	DeleteSessionResultMessage,
	SessionMetadata,
} from "../../types/recording";
import { useRecording } from "../context/RecordingContext";
import "./RecentRecordings.css";

/** How many stored recordings the panel lists (same cap the popup used). */
const RECENT_LIMIT = 5;

/**
 * Recent stored recordings, moved here when the toolbar popup was removed.
 *
 * Deletion goes through the background (`DELETE_SESSION`), never IndexedDB
 * directly, so the UI shares the `assertNotLiveSession` guard with htrcli's
 * `recordingDelete` command and cannot delete the in-flight recording.
 */
export function RecentRecordings() {
	const { pastSessions, refreshPastSessions, session } = useRecording();
	const [deletingId, setDeletingId] = useState<string | null>(null);
	// Tied to the recording it failed for, so it disappears with that row.
	const [error, setError] = useState<{
		sessionId: string;
		message: string;
	} | null>(null);

	const recent = pastSessions.slice(0, RECENT_LIMIT);
	const rowError =
		error && recent.some((recording) => recording.id === error.sessionId)
			? error
			: null;

	const deleteRecording = useCallback(
		async (recording: SessionMetadata) => {
			setDeletingId(recording.id);
			setError(null);
			try {
				const response = (await chrome.runtime.sendMessage({
					type: "DELETE_SESSION",
					sessionId: recording.id,
				})) as DeleteSessionResultMessage | undefined;
				if (!response?.success) {
					setError({
						sessionId: recording.id,
						message: response?.error ?? "Delete failed",
					});
					return;
				}
				// A real deletion is broadcast as DELETE_SESSION_RESULT, and
				// RecordingContext reloads the list on that. The not-found reply goes
				// only back to this caller, so it still needs the local refresh.
				if (!response.deleted) await refreshPastSessions();
			} catch (err) {
				setError({
					sessionId: recording.id,
					message: err instanceof Error ? err.message : String(err),
				});
			} finally {
				setDeletingId(null);
			}
		},
		[refreshPastSessions],
	);

	if (recent.length === 0) return null;

	return (
		<section className="recent-recordings" aria-label="Recent recordings">
			<h4>Recent recordings</h4>
			{rowError && (
				<p className="rr-error" role="alert">
					{rowError.message}
				</p>
			)}
			<ul className="rr-list">
				{recent.map((recording) => {
					// The live session is not persisted yet; deleting it would be
					// refused by the background anyway, so keep the button disabled
					// with the reason in the tooltip.
					const isLive = Boolean(session && session.id === recording.id);
					const busy = deletingId === recording.id;
					return (
						<li key={recording.id} className="rr-item">
							<div className="rr-info">
								<span className="rr-title">{recording.title}</span>
								<span className="rr-meta">
									{new Date(recording.startTime).toLocaleString()} ·{" "}
									{recording.stepCount} steps
								</span>
							</div>
							<button
								type="button"
								className="rr-delete"
								aria-label={`Delete recording ${recording.title}`}
								disabled={isLive || busy}
								title={
									isLive
										? "This recording is in progress — stop it first"
										: "Delete this recording"
								}
								onClick={() => void deleteRecording(recording)}
							>
								{busy ? "…" : "Delete"}
							</button>
						</li>
					);
				})}
			</ul>
		</section>
	);
}

export default RecentRecordings;
