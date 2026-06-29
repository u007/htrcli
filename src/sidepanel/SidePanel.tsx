import { ConnectedTabs } from "./components/ConnectedTabs";
import { ExportPanel } from "./components/ExportPanel";
import { RecordingHeader } from "./components/RecordingHeader";
import { Timeline } from "./components/Timeline";
import { RecordingProvider } from "./context/RecordingContext";
import "./SidePanel.css";
import pkg from "../../package.json";

function SidePanelContent() {
	return (
		<div className="sidepanel">
			<header className="sidepanel-header">
				<h1>How-To Recorder</h1>
				<span className="sidepanel-version">v{pkg.version}</span>
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
