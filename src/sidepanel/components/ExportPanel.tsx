import { useState } from "react";
import { downloadJson } from "../../utils/exportJson";
import { downloadMarkdownWithMedia } from "../../utils/exportMarkdown";
import { exportToZip } from "../../utils/exportZip";
import { useRecording } from "../context/RecordingContext";
import "./ExportPanel.css";

export function ExportPanel() {
	const { session, isRecording } = useRecording();
	const [isExporting, setIsExporting] = useState(false);

	if (!session || isRecording) {
		return null;
	}

	const handleExportJson = () => {
		downloadJson(session);
	};

	const handleExportMarkdown = () => {
		downloadMarkdownWithMedia(session);
	};

	const handleExportZip = async () => {
		setIsExporting(true);
		try {
			await exportToZip(session);
		} catch (error) {
			console.error("Failed to export ZIP:", error);
			alert("Failed to export ZIP file");
		} finally {
			setIsExporting(false);
		}
	};

	return (
		<div className="export-panel">
			<h4>Export Recording</h4>
			<div className="export-buttons">
				<button type="button" className="export-btn" onClick={handleExportJson}>
					<span className="export-icon">📄</span>
					<span className="export-label">JSON</span>
					<span className="export-desc">Raw data</span>
				</button>
				<button
					type="button"
					className="export-btn"
					onClick={handleExportMarkdown}
				>
					<span className="export-icon">📝</span>
					<span className="export-label">Markdown</span>
					<span className="export-desc">Self-contained</span>
				</button>
				<button
					type="button"
					className="export-btn export-btn-primary"
					onClick={handleExportZip}
					disabled={isExporting}
				>
					<span className="export-icon">📦</span>
					<span className="export-label">
						{isExporting ? "Exporting..." : "ZIP Bundle"}
					</span>
					<span className="export-desc">Separate files</span>
				</button>
			</div>
		</div>
	);
}
