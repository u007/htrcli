import React, { useState } from "react";
import type {
	Annotation,
	RecordingStep,
	TimelineItem,
} from "../../types/recording";
import { formatTime, useRecording } from "../context/RecordingContext";
import "./Timeline.css";

export function Timeline() {
	const { session, addAnnotation, updateAnnotation, deleteAnnotation } =
		useRecording();

	if (!session) {
		return (
			<div className="timeline-empty">
				<p>Start a recording to see the timeline</p>
			</div>
		);
	}

	// Merge steps and annotations into a single timeline
	const timelineItems: TimelineItem[] = [
		...session.steps.map(
			(step): TimelineItem => ({ type: "step", data: step }),
		),
		...session.annotations.map(
			(ann): TimelineItem => ({ type: "annotation", data: ann }),
		),
	].sort((a, b) => a.data.timestamp - b.data.timestamp);

	return (
		<div className="timeline">
			{timelineItems.length === 0 ? (
				<div className="timeline-empty">
					<p>Waiting for interactions...</p>
				</div>
			) : (
				<div className="timeline-items">
					{timelineItems.map((item, index) => (
						<React.Fragment key={item.data.id}>
							{item.type === "step" ? (
								<TimelineStepCard step={item.data as RecordingStep} />
							) : (
								<TimelineAnnotationCard
									annotation={item.data as Annotation}
									onUpdate={updateAnnotation}
									onDelete={deleteAnnotation}
								/>
							)}
							{/* Add annotation button between items */}
							<AddAnnotationButton
								timestamp={
									index < timelineItems.length - 1
										? (item.data.timestamp +
												timelineItems[index + 1].data.timestamp) /
											2
										: item.data.timestamp + 1000
								}
								onAdd={addAnnotation}
							/>
						</React.Fragment>
					))}
				</div>
			)}
		</div>
	);
}

// Step card component
interface TimelineStepCardProps {
	step: RecordingStep;
}

function TimelineStepCard({ step }: TimelineStepCardProps) {
	const [showScreenshot, setShowScreenshot] = useState(false);

	const getStepIcon = () => {
		switch (step.type) {
			case "navigation":
				return "🌐";
			case "click":
				return "🖱️";
			case "input":
				return "⌨️";
			default:
				return "•";
		}
	};

	const getStepDescription = () => {
		switch (step.type) {
			case "navigation":
				return `Navigated to ${new URL(step.url).hostname}`;
			case "click":
				if (step.element?.text) {
					return `Clicked "${step.element.text.slice(0, 30)}${step.element.text.length > 30 ? "..." : ""}"`;
				}
				return `Clicked ${step.element?.tag || "element"}`;
			case "input": {
				const fieldName = step.element?.name || step.element?.text || "field";
				if (step.isSensitive) {
					return `Entered ******** in ${fieldName}`;
				}
				return `Entered "${step.inputValue?.slice(0, 20)}${(step.inputValue?.length || 0) > 20 ? "..." : ""}" in ${fieldName}`;
			}
			default:
				return "Unknown action";
		}
	};

	return (
		<div className="timeline-step">
			<div className="step-header">
				<span className="step-time">{formatTime(step.timestamp)}</span>
				<span className="step-icon">{getStepIcon()}</span>
				<span className="step-description">{getStepDescription()}</span>
			</div>

			{step.element?.selector && (
				<div className="step-selector">
					<code>{step.element.selector}</code>
				</div>
			)}

			{step.screenshotData && (
				<div className="step-screenshot">
					<button
						type="button"
						className="screenshot-toggle"
						onClick={() => setShowScreenshot(!showScreenshot)}
					>
						{showScreenshot ? "Hide screenshot" : "Show screenshot"}
					</button>
					{showScreenshot && (
						<img src={step.screenshotData} alt="Step screenshot" />
					)}
				</div>
			)}
		</div>
	);
}

// Annotation card component
interface TimelineAnnotationCardProps {
	annotation: Annotation;
	onUpdate: (id: string, text: string) => Promise<void>;
	onDelete: (id: string) => Promise<void>;
}

function TimelineAnnotationCard({
	annotation,
	onUpdate,
	onDelete,
}: TimelineAnnotationCardProps) {
	const [isEditing, setIsEditing] = useState(false);
	const [editText, setEditText] = useState(annotation.text);

	const handleSave = async () => {
		if (editText.trim()) {
			await onUpdate(annotation.id, editText.trim());
			setIsEditing(false);
		}
	};

	const handleCancel = () => {
		setEditText(annotation.text);
		setIsEditing(false);
	};

	const handleDelete = async () => {
		if (confirm("Delete this annotation?")) {
			await onDelete(annotation.id);
		}
	};

	return (
		<div className="timeline-annotation">
			<div className="annotation-header">
				<span className="step-time">{formatTime(annotation.timestamp)}</span>
				<span className="annotation-icon">📝</span>
				<span className="annotation-label">Note</span>
			</div>

			{isEditing ? (
				<div className="annotation-edit">
					<textarea
						value={editText}
						onChange={(e) => setEditText(e.target.value)}
					/>
					<div className="annotation-buttons">
						<button
							type="button"
							className="btn-small btn-secondary"
							onClick={handleCancel}
						>
							Cancel
						</button>
						<button
							type="button"
							className="btn-small btn-primary"
							onClick={handleSave}
						>
							Save
						</button>
					</div>
				</div>
			) : (
				<>
					<div className="annotation-text">{annotation.text}</div>
					<div className="annotation-actions">
						<button
							type="button"
							className="btn-icon"
							onClick={() => setIsEditing(true)}
							title="Edit"
						>
							✏️
						</button>
						<button
							type="button"
							className="btn-icon"
							onClick={handleDelete}
							title="Delete"
						>
							🗑️
						</button>
					</div>
				</>
			)}
		</div>
	);
}

// Add annotation button
interface AddAnnotationButtonProps {
	timestamp: number;
	onAdd: (text: string, timestamp: number) => Promise<void>;
}

function AddAnnotationButton({ timestamp, onAdd }: AddAnnotationButtonProps) {
	const [isAdding, setIsAdding] = useState(false);
	const [text, setText] = useState("");

	const handleAdd = async () => {
		if (text.trim()) {
			await onAdd(text.trim(), timestamp);
			setText("");
			setIsAdding(false);
		}
	};

	if (isAdding) {
		return (
			<div className="add-annotation-form">
				<textarea
					placeholder="Add a note..."
					value={text}
					onChange={(e) => setText(e.target.value)}
				/>
				<div className="annotation-buttons">
					<button
						type="button"
						className="btn-small btn-secondary"
						onClick={() => setIsAdding(false)}
					>
						Cancel
					</button>
					<button
						type="button"
						className="btn-small btn-primary"
						onClick={handleAdd}
						disabled={!text.trim()}
					>
						Add Note
					</button>
				</div>
			</div>
		);
	}

	return (
		<button
			type="button"
			className="add-annotation-btn"
			onClick={() => setIsAdding(true)}
		>
			+ Add note here
		</button>
	);
}
