import { useEffect, useRef, useState } from "react";
import "./Options.css";

const DEFAULT_SERVER_URL = "ws://127.0.0.1:3845";
const STORAGE_KEYS = ["remoteControlServer", "remoteControlToken"];

type SaveStatus = "idle" | "saving" | "saved" | "error";

export const Options = (): JSX.Element => {
	const [serverUrl, setServerUrl] = useState(DEFAULT_SERVER_URL);
	const [token, setToken] = useState("");
	const [enabled, setEnabled] = useState(false);
	const [saveStatus, setSaveStatus] = useState<SaveStatus>("idle");
	const [saveError, setSaveError] = useState<string | null>(null);
	const successTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

	useEffect(() => {
		chrome.storage.local.get(STORAGE_KEYS, (result) => {
			const lastError = chrome.runtime.lastError;
			if (lastError) {
				console.warn(
					"[How-To Recorder] Failed to load remote control settings:",
					lastError.message,
				);
				return;
			}

			if (
				typeof result.remoteControlServer === "string" &&
				result.remoteControlServer
			) {
				setServerUrl(result.remoteControlServer);
				setEnabled(true);
			}

			if (typeof result.remoteControlToken === "string") {
				setToken(result.remoteControlToken);
			}
		});

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
		await new Promise<void>((resolve, reject) => {
			const finish = (): void => {
				const lastError = chrome.runtime.lastError;
				if (lastError) {
					reject(new Error(lastError.message));
					return;
				}

				resolve();
			};

			if (enabled) {
				chrome.storage.local.set(
					{
						remoteControlServer: serverUrl,
						remoteControlToken: token,
					},
					finish,
				);
				return;
			}

			chrome.storage.local.remove(STORAGE_KEYS, finish);
		});
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
				"[How-To Recorder] Failed to save remote control settings:",
				error,
			);
			setSaveStatus("error");
			setSaveError(message);
		}
	};

	return (
		<main>
			<h3>How-To Recorder Settings</h3>

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
							<input
								type="text"
								value={token}
								disabled={saveStatus === "saving"}
								onChange={(e) => {
									resetFeedback();
									setToken(e.target.value);
								}}
								placeholder="htr_aia_2026"
							/>
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
