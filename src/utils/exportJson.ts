import type {
	ExportedAnnotation,
	ExportedRecording,
	ExportedStep,
	RecordingSession,
} from "../types/recording";

/**
 * Export recording session as JSON
 */
export function exportToJson(session: RecordingSession): ExportedRecording {
	const exportedSteps: ExportedStep[] = session.steps.map((step, index) => ({
		id: step.id,
		timestamp: step.timestamp,
		type: step.type,
		url: step.url,
		tabTitle: step.tabTitle,
		screenshotPath: step.screenshotData
			? `screenshots/step_${index + 1}.png`
			: undefined,
		audioPath: step.audioData ? `audio/step_${index + 1}.webm` : undefined,
		element: step.element,
		inputValue: step.inputValue,
		isSensitive: step.isSensitive,
	}));

	const exportedAnnotations: ExportedAnnotation[] = session.annotations.map(
		(ann, index) => ({
			id: ann.id,
			timestamp: ann.timestamp,
			text: ann.text,
			screenshotPath: ann.screenshotData
				? `screenshots/annotation_${index + 1}.png`
				: undefined,
			audioPath: ann.audioData
				? `audio/annotation_${index + 1}.webm`
				: undefined,
		}),
	);

	return {
		id: session.id,
		title: session.title,
		startTime: session.startTime,
		endTime: session.endTime,
		hasAudio: session.hasAudio,
		steps: exportedSteps,
		annotations: exportedAnnotations,
	};
}

/**
 * Download JSON file
 */
export function downloadJson(session: RecordingSession): void {
	const exported = exportToJson(session);
	const json = JSON.stringify(exported, null, 2);
	const blob = new Blob([json], { type: "application/json" });
	const url = URL.createObjectURL(blob);

	const filename = `${slugify(session.title)}.json`;
	downloadFile(url, filename);
	URL.revokeObjectURL(url);
}

/**
 * Convert string to URL-friendly slug
 */
function slugify(text: string): string {
	return (
		text
			.toLowerCase()
			.trim()
			.replace(/[^\w\s-]/g, "")
			.replace(/[\s_-]+/g, "-")
			.replace(/^-+|-+$/g, "") || "recording"
	);
}

/**
 * Download a file
 */
function downloadFile(url: string, filename: string): void {
	const link = document.createElement("a");
	link.href = url;
	link.download = filename;
	document.body.appendChild(link);
	link.click();
	document.body.removeChild(link);
}
