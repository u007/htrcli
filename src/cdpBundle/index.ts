// Standalone DOM-command bundle for the htcli --cdp transport.
// Built as an IIFE (vite.cdp.config.ts) and embedded in the htcli Go binary
// via go:embed; injected into pages with Runtime.evaluate. Reuses the exact
// selector/actionability/fill engine the extension content script uses, so
// the two transports cannot drift.
import { executeCommand } from "../contentScript/commandExecutor";
import type { Command, CommandResult } from "../types/commands";

declare global {
	interface Window {
		__htcliDom?: {
			exec: (command: Command) => Promise<CommandResult>;
			version: number;
		};
	}
}

window.__htcliDom = {
	exec: (command: Command) => executeCommand(command),
	version: 1,
};
