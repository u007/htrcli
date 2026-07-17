import { ConnectedTabs } from "./components/ConnectedTabs";
import { ExportPanel } from "./components/ExportPanel";
import { RecordingHeader } from "./components/RecordingHeader";
import { Timeline } from "./components/Timeline";
import { RecordingProvider, useRecording } from "./context/RecordingContext";
import "./SidePanel.css";
import pkg from "../../package.json";

function ConnectionIndicator() {
	const { connectionStatus } = useRecording();

	const handleRetry = () => {
		chrome.runtime.sendMessage({ type: "RECONNECT_NATIVE" }).catch(() => {});
	};

	if (connectionStatus === "native") {
		return (
			<span
				className="connection-indicator online"
				title="Server connected (native messaging)"
			>
				<span className="ci-dot" />
				<span className="ci-label">Online</span>
			</span>
		);
	}

	if (connectionStatus === "disconnected") {
		return (
			<span
				className="connection-indicator reconnecting"
				title="Connection lost — retrying automatically"
			>
				<span className="ci-dot" />
				<span className="ci-label">Reconnecting...</span>
			</span>
		);
	}

	// unavailable — native messaging host not available
	return (
		<span className="connection-indicator unavailable">
			<span className="ci-dot" />
			<span className="ci-label">Install htrcli</span>
			<button
				type="button"
				className="ci-retry"
				onClick={handleRetry}
				title="Native messaging host not detected — install htrcli and run 'htrcli serve'"
			>
				↻
			</button>
		</span>
	);
}

function SidePanelContent() {
	return (
		<div className="sidepanel">
			<header className="sidepanel-header">
				<h1>HTR NControl</h1>
				<span className="sidepanel-version">v{pkg.version}</span>
				<ConnectionIndicator />
			</header>
			<ConnectedTabs />
			<RecordingHeader />
			<Timeline />
			<ExportPanel />
		</div>
	);
}

export function SidePanel() {
	return (
		<RecordingProvider>
			<SidePanelContent />
		</RecordingProvider>
	);
}

export default SidePanel;
