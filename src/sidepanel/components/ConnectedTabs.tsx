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
	// Outcome of the last grant attempt, surfaced in the panel because the
	// sidebar console is invisible to most users and the failure reasons
	// (gesture requirement, <all_urls> not requestable, API absent) differ.
	const [grantResult, setGrantResult] = useState<string | null>(null);

	const refresh = useCallback(async (): Promise<void> => {
		try {
			const res = (await chrome.runtime.sendMessage({
				type: "GET_READY_TABS",
			})) as { success?: boolean; tabs?: TabInfo[] } | undefined;
			if (res?.success && Array.isArray(res.tabs)) {
				setTabs(res.tabs);
			}
		} catch (err) {
			console.warn("[HTR NControl] GET_READY_TABS failed:", err);
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
			console.warn("[HTR NControl] permissions.contains failed:", err);
			setHasAccess(true);
		}
	}, []);

	const activateTab = useCallback(
		async (tabId: number): Promise<void> => {
			try {
				const tab = await chrome.tabs.get(tabId);
				await chrome.tabs.update(tabId, { active: true });
				// Also focus the window the tab lives in, otherwise the
				// switch is invisible when the tab is in another window.
				await chrome.windows.update(tab.windowId, { focused: true });
				await refresh();
			} catch (err) {
				console.warn("[HTR NControl] activate tab failed:", err);
			}
		},
		[refresh],
	);

	const requestAccess = useCallback(async (): Promise<void> => {
		if (!chrome.permissions?.request) {
			setGrantResult("permissions.request API unavailable in this context");
			return;
		}
		setRequesting(true);
		setGrantResult(null);
		try {
			const granted = await chrome.permissions.request({
				origins: REMOTE_CONTROL_ORIGINS,
			});
			setHasAccess(granted);
			setGrantResult(
				granted
					? "Access granted — syncing tabs…"
					: `request() returned ${String(granted)} — grant manually: about:addons → this extension → Permissions → "Access your data for all websites"`,
			);
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
			console.warn("[HTR NControl] permissions.request failed:", err);
			setGrantResult(
				`request() failed: ${err instanceof Error ? err.message : String(err)}`,
			);
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
							{grantResult && <span className="ct-hint">{grantResult}</span>}
						</li>
					) : tabs.length === 0 ? (
						<li className="ct-empty">
							<span>No tabs connected.</span>
							<span className="ct-hint">
								Open or reload an http(s) page to connect it. On Firefox, site
								access may still need granting even when it reports as granted —
								use the button below, then reload the page.
							</span>
							<button
								type="button"
								className="ct-grant"
								onClick={() => void requestAccess()}
								disabled={requesting}
							>
								{requesting ? "Requesting…" : "Grant access to tabs"}
							</button>
							{grantResult && <span className="ct-hint">{grantResult}</span>}
						</li>
					) : (
						tabs.map((tab) => (
							<li key={tab.id}>
								<button
									type="button"
									className={`ct-item${tab.active ? " ct-active" : ""}`}
									title="Switch to this tab"
									onClick={() => void activateTab(tab.id)}
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
								</button>
							</li>
						))
					)}
				</ul>
			)}
		</div>
	);
}
