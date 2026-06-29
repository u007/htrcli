import { useCallback, useEffect, useState } from "react";
import "./ConnectedTabs.css";

interface TabInfo {
	id: number;
	url: string;
	title: string;
	active: boolean;
	favIconUrl?: string;
}

export function ConnectedTabs() {
	const [tabs, setTabs] = useState<TabInfo[]>([]);
	const [collapsed, setCollapsed] = useState(false);

	const refresh = useCallback(async (): Promise<void> => {
		try {
			const res = (await chrome.runtime.sendMessage({
				type: "GET_READY_TABS",
			})) as { success?: boolean; tabs?: TabInfo[] } | undefined;
			if (res?.success && Array.isArray(res.tabs)) {
				setTabs(res.tabs);
			}
		} catch {
			// ignore
		}
	}, []);

	useEffect(() => {
		void refresh();
		const id = setInterval(() => {
			void refresh();
		}, 3000);
		return () => clearInterval(id);
	}, [refresh]);

	return (
		<div className="connected-tabs">
			<button
				type="button"
				className="ct-header"
				onClick={() => setCollapsed((c) => !c)}
				onKeyDown={(event) => {
					if (event.key === "Enter" || event.key === " ") {
						event.preventDefault();
						setCollapsed((c) => !c);
					}
				}}
			>
				<span className="ct-title">
					Connected Tabs
					<span className="ct-count">{tabs.length}</span>
				</span>
				<span className="ct-chevron">{collapsed ? "▸" : "▾"}</span>
			</button>

			{!collapsed && (
				<ul className="ct-list">
					{tabs.length === 0 ? (
						<li className="ct-empty">
							<span>No tabs connected.</span>
							<span className="ct-hint">
								If tabs are not visible: go to the nav bar → click the site
								settings icon → disable this extension → re-enable it → then
								refresh the page.
							</span>
						</li>
					) : (
						tabs.map((tab) => (
							<li
								key={tab.id}
								className={`ct-item${tab.active ? " ct-active" : ""}`}
							>
								{tab.favIconUrl ? (
									<img className="ct-favicon" src={tab.favIconUrl} alt="" />
								) : (
									<span className="ct-favicon-placeholder" />
								)}
								<div className="ct-info">
									<span className="ct-name">{tab.title || "(no title)"}</span>
									<span className="ct-id">ID: {tab.id}</span>
								</div>
								{tab.active && <span className="ct-badge">active</span>}
							</li>
						))
					)}
				</ul>
			)}
		</div>
	);
}
