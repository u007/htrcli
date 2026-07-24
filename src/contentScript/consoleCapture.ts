import type { ConsoleEntry } from "../types/recording";

const MESSAGE_SOURCE = "htrncontrol-console-capture";
// Posted by the isolated-world content script (src/contentScript/index.ts)
// once its relay listener is attached. This script runs at document_start
// in the MAIN world, ahead of the isolated-world script's default timing,
// so early console calls are buffered here until that ready signal arrives
// instead of being silently dropped by an unattached window.postMessage
// listener.
const RELAY_READY_TYPE = "HTR_CONSOLE_CAPTURE_RELAY_READY";
const CONSOLE_LEVELS = ["log", "warn", "error", "info", "debug"] as const;
const MAX_BUFFERED_BEFORE_READY = 500;

interface ConsoleCaptureWindow extends Window {
	__htrncontrolConsoleCaptureInitialized?: boolean;
}

const captureWindow = window as ConsoleCaptureWindow;
let relayReady = false;
const pendingBeforeReady: ConsoleEntry[] = [];

function stringifyConsoleArg(arg: unknown): string {
	if (typeof arg === "string") return arg;
	try {
		const json = JSON.stringify(arg);
		if (json !== undefined) return json;
	} catch {
		// Fall through to String() below.
	}
	try {
		return String(arg);
	} catch {
		return "[unserializable]";
	}
}

function getSourceFromStack(stack?: string): string | undefined {
	if (!stack) return undefined;
	const lines = stack.split("\n").slice(2);
	for (const line of lines) {
		const trimmed = line.trim();
		if (!trimmed || trimmed.includes("consoleCapture")) continue;
		return trimmed.replace(/^at\s+/, "");
	}
	return undefined;
}

function sendConsoleEntry(entry: ConsoleEntry): void {
	window.postMessage(
		{
			source: MESSAGE_SOURCE,
			type: "CONSOLE_ENTRY",
			entry,
		},
		"*",
	);
}

function postConsoleEntry(entry: ConsoleEntry): void {
	if (!relayReady) {
		pendingBeforeReady.push(entry);
		if (pendingBeforeReady.length > MAX_BUFFERED_BEFORE_READY) {
			pendingBeforeReady.shift();
		}
		return;
	}
	sendConsoleEntry(entry);
}

window.addEventListener("message", (event) => {
	if (event.source !== window) return;
	const data = event.data as { source?: string; type?: string } | undefined;
	if (data?.source !== MESSAGE_SOURCE || data.type !== RELAY_READY_TYPE) return;
	if (relayReady) return;
	relayReady = true;
	for (const entry of pendingBeforeReady) {
		sendConsoleEntry(entry);
	}
	pendingBeforeReady.length = 0;
});

function wrapConsoleLevel(level: (typeof CONSOLE_LEVELS)[number]): void {
	const original = console[level];
	if (typeof original !== "function") return;
	const bound = original.bind(console);
	console[level] = ((...args: unknown[]) => {
		const entry: ConsoleEntry = {
			level,
			args: args.map(stringifyConsoleArg),
			source: getSourceFromStack(new Error().stack),
		};
		let result: unknown;
		try {
			result = bound(...args);
		} finally {
			postConsoleEntry(entry);
		}
		return result;
	}) as typeof original;
}

if (!captureWindow.__htrncontrolConsoleCaptureInitialized) {
	captureWindow.__htrncontrolConsoleCaptureInitialized = true;
	for (const level of CONSOLE_LEVELS) {
		wrapConsoleLevel(level);
	}
}
