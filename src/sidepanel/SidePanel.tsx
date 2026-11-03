import { type KeyboardEvent, useEffect, useState } from "react";
import { ConnectedTabs, useConnectedTabs } from "./components/ConnectedTabs";
import { ExportPanel } from "./components/ExportPanel";
import { RecentRecordings } from "./components/RecentRecordings";
import { RecordingHeader } from "./components/RecordingHeader";
import { Timeline } from "./components/Timeline";
import { RecordingProvider, useRecording } from "./context/RecordingContext";
import {
	nextSidePanelTab,
	readSidePanelTab,
	type SidePanelTab,
	writeSidePanelTab,
} from "./tabState";
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
	// The panel is the whole UI now (there is no toolbar popup): two views,
	// Recording and Tabs. The selection is remembered so reopening the panel
	// returns to the last view instead of always resetting to Recording.
	const [activeTab, setActiveTab] = useState<SidePanelTab>(readSidePanelTab);
	const connectedTabs = useConnectedTabs();
	const connectedCount = connectedTabs.tabs.length;

	useEffect(() => {
		writeSidePanelTab(activeTab);
	}, [activeTab]);

	// Roving tabindex: Left/Right/Home/End move the selection and focus.
	const handleTabKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
		const next = nextSidePanelTab(activeTab, event.key);
		if (!next) return;
		event.preventDefault();
		setActiveTab(next);
		document.getElementById(`sidepanel-tab-${next}`)?.focus();
	};

	return (
		<div className="sidepanel">
			<header className="sidepanel-header">
				<h1>HTR NControl</h1>
				<span className="sidepanel-version">v{pkg.version}</span>
				<ConnectionIndicator />
			</header>

			<div
				className="sidepanel-tabs"
				role="tablist"
				aria-label="Side panel view"
			>
				<button
					type="button"
					role="tab"
					id="sidepanel-tab-recording"
					aria-selected={activeTab === "recording"}
					aria-controls="sidepanel-panel-recording"
					className="sidepanel-tab"
					tabIndex={activeTab === "recording" ? 0 : -1}
					onClick={() => setActiveTab("recording")}
					onKeyDown={handleTabKeyDown}
				>
					Recording
				</button>
				<button
					type="button"
					role="tab"
					id="sidepanel-tab-tabs"
					aria-selected={activeTab === "tabs"}
					aria-controls="sidepanel-panel-tabs"
					className="sidepanel-tab"
					tabIndex={activeTab === "tabs" ? 0 : -1}
					onClick={() => setActiveTab("tabs")}
					onKeyDown={handleTabKeyDown}
				>
					Tabs
					{connectedCount > 0 && (
						<span className="sidepanel-tab-count">{connectedCount}</span>
					)}
				</button>
			</div>

			{/* Both panels stay mounted: switching views must not drop recording
			    state or reset the tab-registry polling. */}
			<div
				id="sidepanel-panel-recording"
				role="tabpanel"
				aria-labelledby="sidepanel-tab-recording"
				className="sidepanel-panel"
				hidden={activeTab !== "recording"}
			>
				<RecordingHeader />
				<Timeline />
				<RecentRecordings />
				<ExportPanel />
			</div>

			<div
				id="sidepanel-panel-tabs"
				role="tabpanel"
				aria-labelledby="sidepanel-tab-tabs"
				className="sidepanel-panel sidepanel-panel-tabs"
				hidden={activeTab !== "tabs"}
			>
				<ConnectedTabs {...connectedTabs} />
			</div>
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
