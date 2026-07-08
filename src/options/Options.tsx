import { useCallback, useEffect, useRef, useState } from "react";
import "./Options.css";

const DEFAULT_SERVER_URL = "wss://127.0.0.1:3845";
const STORAGE_KEYS = ["remoteControlServer", "remoteControlToken"];

type SaveStatus = "idle" | "saving" | "saved" | "error";

interface TabInfo {
	id: number;
	url: string;
	title: string;
	active: boolean;
	favIconUrl?: string;
}

/**
 * Generate a per-install random token client-side. 32 bytes hex (64 chars)
 * prefixed with `htr_` so it's identifiable. The background script does the
 * same on first install; this fallback is used by the Options page's
 * "Regenerate" button so users can rotate without reloading the extension.
 */
async function generateClientToken(): Promise<string> {
	const bytes = new Uint8Array(32);
	crypto.getRandomValues(bytes);
	const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join(
		"",
	);
	return `htr_${hex}`;
}

export const Options = (): JSX.Element => {
	const [serverUrl, setServerUrl] = useState(DEFAULT_SERVER_URL);
	const [token, setToken] = useState("");
	const [enabled, setEnabled] = useState(true);
	const [saveStatus, setSaveStatus] = useState<SaveStatus>("idle");
	const [saveError, setSaveError] = useState<string | null>(null);
	const [tabs, setTabs] = useState<TabInfo[]>([]);
	const [copyStatus, setCopyStatus] = useState<"idle" | "copied">("idle");
	const successTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

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

	useEffect(() => {
		void (async () => {
			try {
				const result = (await chrome.storage.local.get(STORAGE_KEYS)) as {
					remoteControlServer?: string;
					remoteControlToken?: string;
				};

				if (
					typeof result.remoteControlServer === "string" &&
					result.remoteControlServer
				) {
					setServerUrl(result.remoteControlServer);
				}

				if (
					typeof result.remoteControlToken === "string" &&
					result.remoteControlToken
				) {
					setToken(result.remoteControlToken);
				}
			} catch (error) {
				console.warn(
					"[HTR NControl] Failed to load remote control settings:",
					error,
				);
			}
		})();

		return () => {
			if (successTimeoutRef.current) {
				clearTimeout(successTimeoutRef.current);
			}
		};
	}, []);

	const clearSuccessTimeout = (): void => {
		if (successTimeoutRef.current) {
			clearTimeout(successTimeoutRef.current);
			successTimeoutRef.current = null;
		}
	};

	const resetFeedback = (): void => {
		clearSuccessTimeout();
		setSaveStatus("idle");
		setSaveError(null);
	};

	const scheduleStatusReset = (): void => {
		clearSuccessTimeout();
		successTimeoutRef.current = setTimeout(() => {
			setSaveStatus("idle");
			successTimeoutRef.current = null;
		}, 2000);
	};

	const persistSettings = async (): Promise<void> => {
		if (enabled) {
			await chrome.storage.local.set({
				remoteControlServer: serverUrl,
				remoteControlToken: token,
			});
			return;
		}

		await chrome.storage.local.remove(STORAGE_KEYS);
	};

	const save = async (): Promise<void> => {
		setSaveStatus("saving");
		setSaveError(null);

		try {
			await persistSettings();
			setSaveStatus("saved");
			scheduleStatusReset();
		} catch (error) {
			clearSuccessTimeout();
			const message = error instanceof Error ? error.message : String(error);
			console.warn(
				"[HTR NControl] Failed to save remote control settings:",
				error,
			);
			setSaveStatus("error");
			setSaveError(message);
		}
	};

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

			<section className="section">
				<h4>Remote Control</h4>
				<p className="hint">
					Connect the extension to a local server so external tools can control
					your browser tabs.
				</p>

				<label className="toggle-label">
					<input
						type="checkbox"
						checked={enabled}
						disabled={saveStatus === "saving"}
						onChange={(e) => {
							resetFeedback();
							setEnabled(e.target.checked);
						}}
					/>
					Enable remote control
				</label>

				{enabled && (
					<div className="fields">
						<label>
							Server URL
							<input
								type="text"
								value={serverUrl}
								disabled={saveStatus === "saving"}
								onChange={(e) => {
									resetFeedback();
									setServerUrl(e.target.value);
								}}
								placeholder="ws://127.0.0.1:3845"
							/>
						</label>
						<label>
							Bearer Token
							<div className="token-row">
								<input
									type="text"
									value={token}
									disabled={saveStatus === "saving"}
									onChange={(e) => {
										resetFeedback();
										setToken(e.target.value);
									}}
									placeholder="(generated on first install)"
								/>
								<button
									type="button"
									className="btn-sm"
									onClick={async () => {
										const fresh = await generateClientToken();
										setToken(fresh);
										resetFeedback();
									}}
									disabled={saveStatus === "saving"}
									title="Generate a new random token"
								>
									Regenerate
								</button>
								<button
									type="button"
									className="btn-sm"
									onClick={async () => {
										try {
											await navigator.clipboard.writeText(token);
											setCopyStatus("copied");
											setTimeout(() => setCopyStatus("idle"), 1500);
										} catch {
											// Clipboard may be unavailable (insecure context);
											// the user can still copy manually.
										}
									}}
									disabled={!token || saveStatus === "saving"}
									title="Copy token to clipboard"
								>
									{copyStatus === "copied" ? "Copied!" : "Copy"}
								</button>
							</div>
							<p className="hint">
								Per-install token. Set the same value as{" "}
								<code>HTR_BEARER_TOKEN</code> when starting the server (
								<code>make serve</code> or <code>bun run server</code>) so they
								match. Rotate by clicking Regenerate and updating the server.
							</p>
						</label>
					</div>
				)}

				<button
					type="button"
					onClick={() => void save()}
					disabled={saveStatus === "saving"}
				>
					{saveStatus === "saving"
						? "Saving..."
						: saveStatus === "saved"
							? "Saved!"
							: "Save"}
				</button>

				{saveStatus === "error" && saveError && (
					<p className="status error">Failed to save settings: {saveError}</p>
				)}

				{saveStatus === "saved" && (
					<p className="status success">Settings saved.</p>
				)}

				<p className="hint">
					Reload the extension after saving changes to apply them.
				</p>
			</section>
		</main>
	);
};

export default Options;
