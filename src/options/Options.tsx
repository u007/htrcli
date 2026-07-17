import { useCallback, useEffect, useState } from "react";
import "./Options.css";

interface TabInfo {
	id: number;
	url: string;
	title: string;
	active: boolean;
	favIconUrl?: string;
}

export const Options = (): JSX.Element => {
	const [tabs, setTabs] = useState<TabInfo[]>([]);

	const refreshTabs = useCallback(async (): Promise<void> => {
		try {
			const res = (await chrome.runtime.sendMessage({
				type: "GET_READY_TABS",
			})) as { success?: boolean; tabs?: TabInfo[] } | undefined;
			if (res?.success && Array.isArray(res.tabs)) {
				setTabs(res.tabs);
			}
		} catch (error) {
			console.warn("[HTR NControl] Failed to load connected tabs:", error);
		}
	}, []);

	useEffect(() => {
		void refreshTabs();
		const interval = setInterval(() => {
			void refreshTabs();
		}, 3000);
		return () => clearInterval(interval);
	}, [refreshTabs]);

	return (
		<main>
			<h3>HTR NControl Settings</h3>

			<section className="section tabs-section">
				<div className="tabs-header">
					<h4>Connected Tabs</h4>
					<button type="button" className="btn-sm" onClick={refreshTabs}>
						Refresh
					</button>
				</div>
				{tabs.length === 0 ? (
					<p className="hint">No tabs connected. Refresh a page to connect.</p>
				) : (
					<ul className="tab-list">
						{tabs.map((tab) => (
							<li
								key={tab.id}
								className={`tab-item${tab.active ? " tab-active" : ""}`}
							>
								{tab.favIconUrl && (
									<img className="tab-favicon" src={tab.favIconUrl} alt="" />
								)}
								<div className="tab-info">
									<span className="tab-title">{tab.title || "(no title)"}</span>
									<span className="tab-meta">
										ID: {tab.id} &middot;{" "}
										{tab.url.length > 50 ? `${tab.url.slice(0, 50)}…` : tab.url}
									</span>
								</div>
								{tab.active && <span className="tab-badge">active</span>}
							</li>
						))}
					</ul>
				)}
			</section>
		</main>
	);
};

export default Options;
