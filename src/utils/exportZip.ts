import JSZip from "jszip";
import type { RecordingSession } from "../types/recording";
import { exportToJson } from "./exportJson";
import { generateMarkdown } from "./exportMarkdown";

/**
 * Convert base64 data URL to Blob
 */
function base64ToBlob(base64: string, mimeType: string): Blob {
	// Handle data URL format
	const base64Data = base64.includes(",") ? base64.split(",")[1] : base64;
	const byteCharacters = atob(base64Data);
	const byteNumbers = new Array(byteCharacters.length);

	for (let i = 0; i < byteCharacters.length; i++) {
		byteNumbers[i] = byteCharacters.charCodeAt(i);
	}

	const byteArray = new Uint8Array(byteNumbers);
	return new Blob([byteArray], { type: mimeType });
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
 * Export recording session as ZIP file with all assets
 */
export async function exportToZip(session: RecordingSession): Promise<void> {
	const zip = new JSZip();

	// Add JSON file
	const jsonData = exportToJson(session);
	zip.file("recording.json", JSON.stringify(jsonData, null, 2));

	// Add Markdown file
	const markdown = generateMarkdown(session);
	zip.file("README.md", markdown);

	// Create folders
	const screenshotsFolder = zip.folder("screenshots");
	const audioFolder = zip.folder("audio");

	// Add screenshots and audio from steps
	session.steps.forEach((step, index) => {
		const stepNum = index + 1;

		// Add screenshot
		if (step.screenshotData && screenshotsFolder) {
			try {
				const blob = base64ToBlob(step.screenshotData, "image/png");
				screenshotsFolder.file(`step_${stepNum}.png`, blob);
			} catch (error) {
				console.warn(`Failed to add screenshot for step ${stepNum}:`, error);
			}
		}

		// Add audio
		if (step.audioData && audioFolder) {
			try {
				const blob = base64ToBlob(step.audioData, "audio/webm");
				audioFolder.file(`step_${stepNum}.webm`, blob);
			} catch (error) {
				console.warn(`Failed to add audio for step ${stepNum}:`, error);
			}
		}
	});

	// Add screenshots and audio from annotations
	session.annotations.forEach((annotation, index) => {
		const annNum = index + 1;

		if (annotation.screenshotData && screenshotsFolder) {
			try {
				const blob = base64ToBlob(annotation.screenshotData, "image/png");
				screenshotsFolder.file(`annotation_${annNum}.png`, blob);
			} catch (error) {
				console.warn(
					`Failed to add screenshot for annotation ${annNum}:`,
					error,
				);
			}
		}

		if (annotation.audioData && audioFolder) {
			try {
				const blob = base64ToBlob(annotation.audioData, "audio/webm");
				audioFolder.file(`annotation_${annNum}.webm`, blob);
			} catch (error) {
				console.warn(`Failed to add audio for annotation ${annNum}:`, error);
			}
		}
	});

	// Generate the ZIP file
	const zipBlob = await zip.generateAsync({
		type: "blob",
		compression: "DEFLATE",
		compressionOptions: { level: 6 },
	});

	// Download the ZIP
	const url = URL.createObjectURL(zipBlob);
	const filename = `${slugify(session.title)}.zip`;

	const link = document.createElement("a");
	link.href = url;
	link.download = filename;
	document.body.appendChild(link);
	link.click();
	document.body.removeChild(link);

	URL.revokeObjectURL(url);
}
