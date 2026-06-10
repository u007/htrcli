import { useState } from "react";
import { formatTime, useRecording } from "../context/RecordingContext";
import "./RecordingHeader.css";

export function RecordingHeader() {
	const {
		isRecording,
		session,
		elapsedTime,
		isLoading,
		startRecording,
		stopRecording,
		clearSession,
	} = useRecording();
	const [title, setTitle] = useState("");
	const [showStartDialog, setShowStartDialog] = useState(false);
	const [hasAudio, setHasAudio] = useState(false);

	const handleStartClick = () => {
		if (session && !isRecording) {
			// Clear current session and start fresh
			clearSession();
		}
		setShowStartDialog(true);
	};

	const handleStart = async () => {
		if (!title.trim()) return;
		await startRecording(title.trim(), hasAudio);
		setShowStartDialog(false);
		setTitle("");
	};

	const handleStop = async () => {
		console.log("[RecordingHeader] Stop button clicked, isLoading:", isLoading);
		await stopRecording();
		console.log("[RecordingHeader] Stop recording completed");
	};

	const handleCancel = () => {
		setShowStartDialog(false);
		setTitle("");
	};

	if (showStartDialog) {
		return (
			<div className="recording-header">
				<div className="start-dialog">
					<h3>New Recording</h3>
					<input
						type="text"
						placeholder="Recording title..."
						value={title}
						onChange={(e) => setTitle(e.target.value)}
						onKeyDown={(e) => e.key === "Enter" && handleStart()}
					/>
					<label className="audio-toggle">
						<input
							type="checkbox"
							checked={hasAudio}
							onChange={(e) => setHasAudio(e.target.checked)}
						/>
						Enable audio narration
					</label>
					<div className="dialog-buttons">
						<button
							type="button"
							className="btn-secondary"
							onClick={handleCancel}
						>
							Cancel
						</button>
						<button
							type="button"
							className="btn-primary"
							onClick={handleStart}
							disabled={!title.trim()}
						>
							Start Recording
						</button>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="recording-header">
			{isRecording ? (
				<>
					<div className="recording-info">
						<span className="recording-indicator">REC</span>
						<span className="recording-title">{session?.title}</span>
					</div>
					<div className="recording-controls">
						<span className="elapsed-time">{formatTime(elapsedTime)}</span>
						<button
							type="button"
							className="btn-stop"
							onClick={handleStop}
							disabled={isLoading}
						>
							{isLoading ? "Stopping..." : "Stop Recording"}
						</button>
					</div>
				</>
			) : session ? (
				<>
					<div className="recording-info">
						<span className="recording-title">{session.title}</span>
						<span className="step-count">{session.steps.length} steps</span>
					</div>
					<div className="recording-controls">
						<button
							type="button"
							className="btn-primary"
							onClick={handleStartClick}
						>
							New Recording
						</button>
					</div>
				</>
			) : (
				<div className="no-recording">
					<button
						type="button"
						className="btn-primary btn-large"
						onClick={handleStartClick}
					>
						Start New Recording
					</button>
				</div>
			)}
		</div>
	);
}
