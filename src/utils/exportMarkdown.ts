import type {
	Annotation,
	RecordingSession,
	RecordingStep,
	TimelineItem,
} from "../types/recording";

/**
 * Format timestamp as MM:SS
 */
function formatTime(ms: number): string {
	const totalSeconds = Math.floor(ms / 1000);
	const minutes = Math.floor(totalSeconds / 60);
	const seconds = totalSeconds % 60;
	return `${minutes.toString().padStart(2, "0")}:${seconds.toString().padStart(2, "0")}`;
}

/**
 * Format date for display
 */
function formatDate(timestamp: number): string {
	const date = new Date(timestamp);
	return date.toLocaleDateString("en-US", {
		year: "numeric",
		month: "long",
		day: "numeric",
	});
}

/**
 * Get step description
 */
function getStepDescription(step: RecordingStep): string {
	switch (step.type) {
		case "navigation":
			return `Navigated to page`;
		case "click":
			if (step.element?.text) {
				return `Clicked "${step.element.text.slice(0, 50)}${step.element.text.length > 50 ? "..." : ""}" ${step.element.tag}`;
			}
			return `Clicked ${step.element?.tag || "element"}`;
		case "input": {
			const fieldName = step.element?.name || step.element?.text || "field";
			if (step.isSensitive) {
				return `Entered sensitive data in ${fieldName}`;
			}
			return `Entered "${step.inputValue}" in ${fieldName}`;
		}
		default:
			return "Unknown action";
	}
}

/**
 * Options for markdown generation
 */
export interface MarkdownOptions {
	/** Embed screenshots and audio as base64 data URLs (default: false) */
	embedMedia?: boolean;
}

/**
 * Generate markdown content for a recording session
 */
export function generateMarkdown(
	session: RecordingSession,
	options: MarkdownOptions = {},
): string {
	const { embedMedia = false } = options;
	const lines: string[] = [];

	// Title
	lines.push(`# ${session.title}`);
	lines.push("");
	lines.push(`_Recorded on ${formatDate(session.startTime)}_`);
	lines.push("");
	lines.push("---");
	lines.push("");

	// Merge steps and annotations into timeline
	const timelineItems: TimelineItem[] = [
		...session.steps.map(
			(step): TimelineItem => ({ type: "step", data: step }),
		),
		...session.annotations.map(
			(ann): TimelineItem => ({ type: "annotation", data: ann }),
		),
	].sort((a, b) => a.data.timestamp - b.data.timestamp);

	let stepNumber = 0;
	let noteNumber = 0;

	for (const item of timelineItems) {
		if (item.type === "step") {
			stepNumber++;
			const step = item.data as RecordingStep;

			lines.push(
				`## Step ${stepNumber}: ${getStepDescription(step)} (${formatTime(step.timestamp)})`,
			);
			lines.push("");

			// URL
			lines.push(`**URL:** ${step.url}`);
			lines.push("");

			// Element details for clicks/inputs
			if (step.element && step.type !== "navigation") {
				lines.push(
					`**Element:** \`<${step.element.tag}>\`${step.element.text ? ` - "${step.element.text.slice(0, 50)}"` : ""}`,
				);
				lines.push("");
				// Hide selector in collapsible details
				lines.push("<details>");
				lines.push("<summary>Technical Details</summary>");
				lines.push("");
				lines.push(`**Selector:** \`${step.element.selector}\``);
				if (step.element.id) {
					lines.push(`**ID:** \`${step.element.id}\``);
				}
				if (step.element.className) {
					lines.push(`**Classes:** \`${step.element.className}\``);
				}
				if (step.element.name) {
					lines.push(`**Name:** \`${step.element.name}\``);
				}
				lines.push("");
				lines.push("</details>");
				lines.push("");
			}

			// Input value
			if (step.type === "input" && step.inputValue) {
				if (step.isSensitive) {
					lines.push(`**Value:** \\*\\*\\*\\*\\*\\*\\*\\* (sensitive)`);
				} else {
					lines.push(`**Value:** "${step.inputValue}"`);
				}
				lines.push("");
			}

			// Screenshot
			if (embedMedia && step.screenshotData) {
				// Embed as base64 data URL
				lines.push(`![Screenshot](${step.screenshotData})`);
			} else {
				// Reference external file
				lines.push(`![Screenshot](./screenshots/step_${stepNumber}.png)`);
			}
			lines.push("");

			// Audio (if present)
			if (step.audioData) {
				if (embedMedia) {
					// Embed audio as HTML audio element with base64 source
					lines.push(`<audio controls src="${step.audioData}">`);
					lines.push(`  Your browser does not support the audio element.`);
					lines.push(`</audio>`);
				} else {
					lines.push(`[Listen to audio](./audio/step_${stepNumber}.webm)`);
				}
				lines.push("");
			}

			lines.push("---");
			lines.push("");
		} else {
			noteNumber++;
			const annotation = item.data as Annotation;

			lines.push(`## Note ${noteNumber} (${formatTime(annotation.timestamp)})`);
			lines.push("");
			lines.push(`> ${annotation.text}`);
			lines.push("");

			// Annotation screenshot (if present)
			if (annotation.screenshotData) {
				if (embedMedia) {
					lines.push(`![Annotation Screenshot](${annotation.screenshotData})`);
				} else {
					lines.push(
						`![Annotation Screenshot](./screenshots/note_${noteNumber}.png)`,
					);
				}
				lines.push("");
			}

			// Annotation audio (if present)
			if (annotation.audioData) {
				if (embedMedia) {
					lines.push(`<audio controls src="${annotation.audioData}">`);
					lines.push(`  Your browser does not support the audio element.`);
					lines.push(`</audio>`);
				} else {
					lines.push(`[Listen to audio](./audio/note_${noteNumber}.webm)`);
				}
				lines.push("");
			}

			lines.push("---");
			lines.push("");
		}
	}

	return lines.join("\n");
}

/**
 * Download markdown file (with external file references)
 */
export function downloadMarkdown(session: RecordingSession): void {
	const markdown = generateMarkdown(session, { embedMedia: false });
	const blob = new Blob([markdown], { type: "text/markdown" });
	const url = URL.createObjectURL(blob);

	const filename = `${slugify(session.title)}.md`;
	downloadFile(url, filename);
	URL.revokeObjectURL(url);
}

/**
 * Download self-contained markdown file with embedded media
 */
export function downloadMarkdownWithMedia(session: RecordingSession): void {
	const markdown = generateMarkdown(session, { embedMedia: true });
	const blob = new Blob([markdown], { type: "text/markdown" });
	const url = URL.createObjectURL(blob);

	const filename = `${slugify(session.title)}-complete.md`;
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
