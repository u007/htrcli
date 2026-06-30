import { useCallback, useEffect, useState } from "react";
import "./ConnectedTabs.css";

interface TabInfo {
	id: number;
	url: string;
	title: string;
	active: boolean;
	favIconUrl?: string;
}

// The host access the content scripts need in order to connect tabs for
// remote control. On Chrome this is granted at install time; on Firefox
// MV3 `<all_urls>` is opt-in, so the extension has to request it from a
// user gesture before any tab can connect.
const REMOTE_CONTROL_ORIGINS = ["<all_urls>"];

export function ConnectedTabs() {
	const [tabs, setTabs] = useState<TabInfo[]>([]);
	const [collapsed, setCollapsed] = useState(false);
	// `null` while we haven't checked yet. `true`/`false` once known.
	const [hasAccess, setHasAccess] = useState<boolean | null>(null);
	const [requesting, setRequesting] = useState(false);

	const refresh = useCallback(async (): Promise<void> => {
		try {
			const res = (await chrome.runtime.sendMessage({
				type: "GET_READY_TABS",
			})) as { success?: boolean; tabs?: TabInfo[] } | undefined;
			if (res?.success && Array.isArray(res.tabs)) {
				setTabs(res.tabs);
			}
		} catch (err) {
			console.warn("[How-To Recorder] GET_READY_TABS failed:", err);
		}
	}, []);

	const checkAccess = useCallback(async (): Promise<void> => {
		// `chrome.permissions` is unavailable in some contexts; treat its
		// absence as "access granted" so we don't block Chrome users.
		if (!chrome.permissions?.contains) {
			setHasAccess(true);
			return;
		}
		try {
			const granted = await chrome.permissions.contains({
				origins: REMOTE_CONTROL_ORIGINS,
			});
			setHasAccess(granted);
		} catch (err) {
			console.warn("[How-To Recorder] permissions.contains failed:", err);
			setHasAccess(true);
		}
	}, []);

	const requestAccess = useCallback(async (): Promise<void> => {
		if (!chrome.permissions?.request) return;
		setRequesting(true);
		try {
			const granted = await chrome.permissions.request({
				origins: REMOTE_CONTROL_ORIGINS,
			});
			setHasAccess(granted);
			if (granted) {
				// Connect tabs that were already open before access was
				// granted, then show the refreshed list.
				const res = (await chrome.runtime.sendMessage({
					type: "SYNC_READY_TABS",
				})) as { success?: boolean; tabs?: TabInfo[] } | undefined;
				if (res?.success && Array.isArray(res.tabs)) {
					setTabs(res.tabs);
				} else {
					await refresh();
				}
			}
		} catch (err) {
			// User dismissed the prompt or the API is unavailable.
			console.warn("[How-To Recorder] permissions.request failed:", err);
		} finally {
			setRequesting(false);
		}
	}, [refresh]);

	useEffect(() => {
		void checkAccess();
		// On mount, do a full sync so already-running content scripts are detected
		// after a background restart (which clears readyTabs).
		void chrome.runtime
			.sendMessage({ type: "SYNC_READY_TABS" })
			.then((res: { success?: boolean; tabs?: TabInfo[] } | undefined) => {
				if (res?.success && Array.isArray(res.tabs)) setTabs(res.tabs);
			})
			.catch(() => void refresh());
		const id = setInterval(() => {
			void refresh();
		}, 3000);
		return () => clearInterval(id);
	}, [checkAccess, refresh]);

	const needsAccess = hasAccess === false;

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
					{needsAccess ? (
						<li className="ct-empty">
							<span>This extension needs access to your tabs.</span>
							<span className="ct-hint">
								Firefox doesn't grant site access automatically. Click below and
								allow access to all sites so tabs can connect for remote
								control.
							</span>
							<button
								type="button"
								className="ct-grant"
								onClick={() => void requestAccess()}
								disabled={requesting}
							>
								{requesting ? "Requesting…" : "Grant access to tabs"}
							</button>
						</li>
					) : tabs.length === 0 ? (
						<li className="ct-empty">
							<span>No tabs connected.</span>
							<span className="ct-hint">
								Open or reload an http(s) page to connect it. If tabs still
								aren't visible: go to the nav bar → click the site settings icon
								→ disable this extension → re-enable it → then refresh the page.
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
