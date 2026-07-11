/**
 * HTR NControl Server
 *
 * HTTP + WebSocket API server for remote controlling browser tabs.
 * The Chrome extension connects to this server via WebSocket,
 * and external tools send HTTP requests to control the browser.
 *
 * Usage:
 *   bun run dev          # Development with hot reload
 *   bun run start        # Production
 *
 * Environment Variables:
 *   HTR_PORT              - HTTP server port (default: 3845)
 *   HTR_HOST              - HTTP server host (default: 127.0.0.1)
 *   HTR_ENABLE_IP_WHITELIST - Enable IP whitelist (default: true)
 *   HTR_ALLOWED_IPS       - Comma-separated allowed IPs (default: 127.0.0.1,localhost,::1)
 *   HTR_ENABLE_BEARER_TOKEN - Enable bearer token auth (default: true)
 *   HTR_BEARER_TOKEN      - Required bearer token (auto-generated if not set)
 */

import type { ServerWebSocket } from "bun";
import {
	type AuthConfig,
	authorize,
	authorizeWs,
	loadAuthConfig,
} from "./auth";
import { loadEnv } from "./loadEnv";
import type {
	ApiResponse,
	Command,
	CommandAction,
	CommandRequest,
	CommandResult,
	ExtensionMessage,
	PageInfo,
	ServerMessage,
	TabInfo,
} from "./types";

// Navigation actions destroy the content script mid-flight: it acks the command
// before the page unloads, so the raw command_result arrives instantly (~0ms)
// while the page is still loading. For these actions the server must wait for
// the navigation to actually finish (see waitForNavigation below).
const NAV_ACTIONS = new Set(["navigate", "reload", "goBack", "goForward"]);

// Internal action used by the server to route a user's `evaluate` request
// through the content script → background → CDP path. Mirrors the matching
// `evaluateViaCdp` entry in `src/types/commands.ts`. Kept as a typed const
// so a future rename on either side fails the TypeScript build rather than
// silently breaking the dispatch.
const EVALUATE_VIA_CDP: CommandAction = "evaluateViaCdp";

// ─── Configuration ─────────────────────────────────────────────────

// Load repo-root .env files (cwd is `server/` when launched via `bun run
// server`, so Bun's automatic .env loading won't find them otherwise).
loadEnv();

const PORT = Number(process.env.HTR_PORT) || 3845;
const HOST = process.env.HTR_HOST || "127.0.0.1";

// TLS cert paths (relative to this file)
const CERT_PATH =
	process.env.HTR_CERT || new URL("cert.pem", import.meta.url).pathname;
const KEY_PATH =
	process.env.HTR_KEY || new URL("key.pem", import.meta.url).pathname;

import { existsSync, readFileSync } from "node:fs";

const TLS_ENABLED = existsSync(CERT_PATH) && existsSync(KEY_PATH);
const tlsConfig = TLS_ENABLED
	? { cert: readFileSync(CERT_PATH), key: readFileSync(KEY_PATH) }
	: undefined;

// ─── Types ─────────────────────────────────────────────────────────

interface WebSocketData {
	ip: string;
}

// ─── State ─────────────────────────────────────────────────────────

/** Connected extension tabs */
const connectedTabs = new Map<
	number,
	{
		ws: ServerWebSocket<WebSocketData>;
		tabInfo: TabInfo;
		lastHeartbeat: number;
		/** When this tab's content script registered — a registration newer than
		 * a navigation command means the page reloaded (navigation committed). */
		registeredAt: number;
	}
>();

/** Pending commands waiting for response */
const pendingCommands = new Map<
	string,
	{
		resolve: (result: CommandResult) => void;
		timeout: ReturnType<typeof setTimeout>;
		tabId: number;
	}
>();

// Load auth config
const authConfig: AuthConfig = loadAuthConfig();

// If bearer token was auto-generated, log it
if (!process.env.HTR_BEARER_TOKEN && authConfig.enableBearerToken) {
	console.log(`🔑 Auto-generated bearer token: ${authConfig.bearerToken}`);
	console.log(`   Set HTR_BEARER_TOKEN env var to use a custom token`);
}

// ─── WebSocket Handlers (Bun native) ──────────────────────────────

function handleWsOpen(ws: ServerWebSocket<WebSocketData>): void {
	console.log(`🔌 Extension connected from ${ws.data.ip}`);
}

function handleWsMessage(
	ws: ServerWebSocket<WebSocketData>,
	data: string | Buffer,
): void {
	try {
		const raw = data.toString();
		const message = JSON.parse(raw) as ExtensionMessage;
		console.log(
			`📨 WS message received: ${message.type} (${raw.length} bytes)`,
		);
		handleExtensionMessage(ws, message);
	} catch (error) {
		console.error(
			`Failed to parse extension message (${data.toString().length} bytes):`,
			error,
		);
	}
}

function handleWsClose(ws: ServerWebSocket<WebSocketData>): void {
	let found = false;
	for (const [tabId, conn] of connectedTabs.entries()) {
		if (conn.ws === ws) {
			found = true;
			connectedTabs.delete(tabId);
			console.log(`🔌 Tab ${tabId} disconnected`);
			// Resolve pending commands for this tab so callers don't wait for the timeout
			for (const [cmdId, pending] of pendingCommands.entries()) {
				if (pending.tabId === tabId) {
					clearTimeout(pending.timeout);
					pending.resolve({
						id: cmdId,
						success: false,
						error: "Tab disconnected",
					});
					pendingCommands.delete(cmdId);
				}
			}
			break;
		}
	}
	if (!found) {
		console.log(`🔌 Unregistered WS closed (no register msg received)`);
	}
}

function handleExtensionMessage(
	ws: ServerWebSocket<WebSocketData>,
	message: ExtensionMessage,
): void {
	switch (message.type) {
		case "register": {
			// Extension tab is registering itself
			if (message.tabId && message.tabInfo) {
				connectedTabs.set(message.tabId, {
					ws,
					tabInfo: message.tabInfo,
					lastHeartbeat: Date.now(),
					registeredAt: Date.now(),
				});
				console.log(
					`📋 Tab registered: ${message.tabId} - ${message.tabInfo.title} (${message.tabInfo.url})`,
				);
			}
			break;
		}

		case "command_result": {
			// Response to a command
			if (message.commandId && message.result) {
				const pending = pendingCommands.get(message.commandId);
				if (pending) {
					clearTimeout(pending.timeout);
					pendingCommands.delete(message.commandId);
					pending.resolve(message.result);
				}
			}
			break;
		}

		case "heartbeat": {
			// Update last heartbeat
			if (message.tabId) {
				const conn = connectedTabs.get(message.tabId);
				if (conn) {
					conn.lastHeartbeat = Date.now();
				}
			}
			break;
		}

		case "error": {
			console.error("Extension error:", message.error);
			break;
		}
	}
}

// ─── IP Extraction ─────────────────────────────────────────────────

// ─── HTTP Handlers ─────────────────────────────────────────────────

function jsonResponse<T>(
	data: T,
	status = 200,
	headers: Record<string, string> = {},
): Response {
	return new Response(JSON.stringify(data), {
		status,
		headers: {
			"Content-Type": "application/json",
			"Access-Control-Allow-Origin": getAllowOrigin(),
			"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
			"Access-Control-Allow-Headers": "Content-Type, Authorization",
			...headers,
		},
	});
}

function errorResponse(error: string, status = 400): Response {
	return jsonResponse<ApiResponse>({ ok: false, error }, status);
}

/** Determine the CORS origin header based on the request origin */
function getAllowOrigin(): string {
	// In production, restrict to localhost origins only.
	// This is checked per-request by the auth middleware.
	return "*";
}

function handleCorsHeaders(): Record<string, string> {
	return {
		"Access-Control-Allow-Origin": "*",
		"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
		"Access-Control-Allow-Private-Network": "true",
	};
}

async function handleRequest(
	req: Request,
	server: ReturnType<typeof Bun.serve>,
): Promise<Response> {
	const url = new URL(req.url);
	const path = url.pathname;
	const method = req.method;
	const clientIp = extractClientIpFromRequest(req, server);

	// Handle CORS preflight
	if (method === "OPTIONS") {
		return new Response(null, {
			status: 204,
			headers: handleCorsHeaders(),
		});
	}

	// Authorize request (using real client IP instead of hardcoded localhost)
	const authResult = authorize(clientIp, req.headers, authConfig);
	if (!authResult.ok) {
		return errorResponse(authResult.error, 403);
	}

	// ─── Routes ──────────────────────────────────────────────────

	// GET /api/tabs - List connected tabs
	if (path === "/api/tabs" && method === "GET") {
		const tabs = Array.from(connectedTabs.values()).map((c) => c.tabInfo);
		return jsonResponse<ApiResponse<TabInfo[]>>({
			ok: true,
			data: tabs,
		});
	}

	// GET /api/tabs/:id - Get specific tab info
	const tabMatch = path.match(/^\/api\/tabs\/(\d+)$/);
	if (tabMatch && method === "GET") {
		const tabId = Number(tabMatch[1]);
		const conn = connectedTabs.get(tabId);
		if (!conn) {
			return errorResponse(`Tab ${tabId} not connected`, 404);
		}
		return jsonResponse<ApiResponse<TabInfo>>({
			ok: true,
			data: conn.tabInfo,
		});
	}

	// POST /api/tabs/:id/command - Execute command on tab
	const cmdMatch = path.match(/^\/api\/tabs\/(\d+)\/command$/);
	if (cmdMatch && method === "POST") {
		const tabId = Number(cmdMatch[1]);
		const body = (await req.json()) as CommandRequest;

		if (!body.command) {
			return errorResponse("Missing 'command' in request body");
		}

		if (!body.command.id) {
			body.command.id = generateCommandId();
		}

		const result = await sendCommandToTab(tabId, body.command);
		if (!result) {
			return errorResponse(`Tab ${tabId} not connected`, 404);
		}

		return jsonResponse<ApiResponse<CommandResult>>({
			ok: true,
			data: result,
		});
	}

	// POST /api/command - Execute command on active tab
	if (path === "/api/command" && method === "POST") {
		const body = (await req.json()) as CommandRequest & { tabId?: number };

		if (!body.command) {
			return errorResponse("Missing 'command' in request body");
		}

		if (!body.command.id) {
			body.command.id = generateCommandId();
		}

		// Use specified tab or first connected tab
		const tabId = body.tabId || getFirstTabId();
		if (!tabId) {
			return errorResponse("No tabs connected", 404);
		}

		const result = await sendCommandToTab(tabId, body.command);
		if (!result) {
			return errorResponse(`Tab ${tabId} not connected`, 404);
		}

		return jsonResponse<ApiResponse<CommandResult>>({
			ok: true,
			data: result,
		});
	}

	// GET /api/page - Get page info for active tab
	if (path === "/api/page" && method === "GET") {
		const tabId = getFirstTabId();
		if (!tabId) {
			return errorResponse("No tabs connected", 404);
		}

		const result = await sendCommandToTab(tabId, {
			id: generateCommandId(),
			action: "getPageInfo",
		});

		return jsonResponse<ApiResponse<PageInfo>>({
			ok: true,
			data: result?.data as PageInfo,
		});
	}

	// GET /api/screenshot - Take screenshot of active tab (synchronous)
	if (path === "/api/screenshot" && method === "GET") {
		const tabId = getFirstTabId();
		if (!tabId) {
			return errorResponse("No tabs connected", 404);
		}

		// Use sendCommandToTab which waits for the result
		const result = await sendCommandToTab(tabId, {
			id: generateCommandId(),
			action: "screenshot",
		});

		return jsonResponse<ApiResponse>({
			ok: true,
			data: result?.data ?? "Screenshot capture requested",
		});
	}

	// GET /api/health - Health check
	if (path === "/api/health" && method === "GET") {
		return jsonResponse({
			ok: true,
			data: {
				status: "running",
				connectedTabs: connectedTabs.size,
				uptime: process.uptime(),
			},
		});
	}

	// 404
	return errorResponse("Not found", 404);
}

/** Extract client IP from a Bun HTTP request, using the real socket address as authoritative source */
function extractClientIpFromRequest(
	req: Request,
	server: ReturnType<typeof Bun.serve>,
): string {
	// Use Bun's API for the authoritative remote address
	const socketAddr = server.requestIP(req);
	if (socketAddr?.address) return socketAddr.address;

	// Proxied requests may carry the original IP in standard headers
	const forwarded = req.headers.get("x-forwarded-for");
	if (forwarded) {
		const firstIp = forwarded.split(",")[0]?.trim();
		if (firstIp) return firstIp;
	}

	const realIp = req.headers.get("x-real-ip");
	if (realIp) return realIp;

	return "127.0.0.1";
}

// ─── Command Forwarding ────────────────────────────────────────────

const DEFAULT_COMMAND_TIMEOUT = 30000; // 30 seconds

async function sendCommandToTab(
	tabId: number,
	command: Command,
	timeoutMs = DEFAULT_COMMAND_TIMEOUT,
): Promise<CommandResult | null> {
	if (!connectedTabs.has(tabId)) return null;
	// Route `evaluate` through the content-script→background→CDP path. This
	// gives the script the page's main world (vs. the content script's
	// isolated world) and bypasses the MV3 CSP that blocks `new Function`.
	// On Firefox the background's CDP_EVAL handler returns an explicit error
	// and the user sees it; no silent fallback.
	if (command.action === "evaluate") {
		return dispatchCommand(
			tabId,
			{ ...command, action: EVALUATE_VIA_CDP },
			timeoutMs,
		);
	}
	if (!NAV_ACTIONS.has(command.action)) {
		return dispatchCommand(tabId, command, timeoutMs);
	}

	// ─── Navigation: dispatch, then wait for the page to finish loading ──
	const startedAt = Date.now();

	// Snapshot the real current URL (tabInfo.url can be stale after SPA routing)
	const before = await dispatchCommand(
		tabId,
		{ id: generateCommandId(), action: "getPageInfo" },
		2000,
	);
	if (!before?.success) {
		return {
			id: command.id,
			success: false,
			error: `Cannot ${command.action}: page is not responding (${before?.error ?? "no result"})`,
		};
	}
	const urlBefore = (before.data as PageInfo).url;

	const result = await dispatchCommand(tabId, command, timeoutMs);
	if (!result) return null;
	// The content script acks before the page unloads; "Tab disconnected" just
	// means teardown won the race — both mean the navigation was initiated.
	if (!result.success && result.error !== "Tab disconnected") {
		return result;
	}

	// goBack/goForward no-op fast-path: window.history.back()/forward() return
	// instantly even when there's no history. If the URL is unchanged ~1s
	// later, the action was a silent no-op — fail loudly rather than wait the
	// full 25s for a load that will never come.
	if (command.action === "goBack" || command.action === "goForward") {
		await Bun.sleep(1000);
		const after = await dispatchCommand(
			tabId,
			{ id: generateCommandId(), action: "getPageInfo" },
			2000,
		);
		const afterUrl = after?.success ? (after.data as PageInfo).url : undefined;
		if (afterUrl === urlBefore) {
			return {
				id: command.id,
				success: false,
				error:
					command.action === "goBack"
						? "No previous page in this tab's history"
						: "No forward page in this tab's history",
				duration: Date.now() - startedAt,
			};
		}
	}

	return waitForNavigation(tabId, command, urlBefore, startedAt, timeoutMs);
}

/**
 * Wait until a navigation initiated at `startedAt` has finished loading.
 *
 * Completion is detected by polling getPageInfo on the tab until
 * document.readyState === "complete" AND either:
 *   - the tab re-registered after the command (full page load → new content
 *     script → new register message), covering reload/goBack/same-URL loads, or
 *   - the URL changed from the pre-navigation URL, covering same-document
 *     navigations (hash / history API) that never reload the content script.
 *
 * Requires an extension build that reports pageInfo.readyState — older builds
 * will time out here (rebuild + reload the extension).
 */
async function waitForNavigation(
	tabId: number,
	command: Command,
	urlBefore: string,
	startedAt: number,
	timeoutMs: number,
): Promise<CommandResult> {
	// Stay under htrcli's 30s HTTP client timeout so callers get a clean error
	const deadline = startedAt + Math.min(timeoutMs, 25000);

	while (Date.now() < deadline) {
		const conn = connectedTabs.get(tabId);
		if (conn) {
			const reRegistered = conn.registeredAt > startedAt;
			const info = await dispatchCommand(
				tabId,
				{ id: generateCommandId(), action: "getPageInfo" },
				2000,
			);
			const page = info?.success ? (info.data as PageInfo) : undefined;
			if (
				page?.readyState === "complete" &&
				(reRegistered || page.url !== urlBefore)
			) {
				return {
					id: command.id,
					success: true,
					data: { url: page.url, title: page.title },
					duration: Date.now() - startedAt,
					pageInfo: page,
				};
			}
		}
		await Bun.sleep(400);
	}

	return {
		id: command.id,
		success: false,
		error: `${command.action}: page did not finish loading within ${Math.min(timeoutMs, 25000)}ms`,
		duration: Date.now() - startedAt,
	};
}

function dispatchCommand(
	tabId: number,
	command: Command,
	timeoutMs: number,
): Promise<CommandResult> | null {
	const conn = connectedTabs.get(tabId);
	if (!conn) return null;

	return new Promise((resolve) => {
		const timeout = setTimeout(() => {
			pendingCommands.delete(command.id);
			resolve({
				id: command.id,
				success: false,
				error: `Command timed out after ${timeoutMs}ms`,
			});
		}, timeoutMs);

		pendingCommands.set(command.id, {
			resolve,
			timeout,
			tabId,
		});

		// Send command to extension
		const msg: ServerMessage = {
			type: "command",
			tabId,
			command,
			timestamp: Date.now(),
		};
		conn.ws.send(JSON.stringify(msg));
	});
}

function getFirstTabId(): number | null {
	for (const tabId of connectedTabs.keys()) {
		return tabId;
	}
	return null;
}

function generateCommandId(): string {
	return `cmd_${Date.now()}_${Math.random().toString(36).substring(2, 8)}`;
}

// ─── Stale Connection Cleanup + Ping ──────────────────────────────

setInterval(() => {
	const now = Date.now();
	const staleThreshold = 60000; // 60 seconds
	const pingMsg = JSON.stringify({
		type: "ping",
		timestamp: now,
	} satisfies ServerMessage);

	for (const [tabId, conn] of connectedTabs.entries()) {
		if (now - conn.lastHeartbeat > staleThreshold) {
			console.log(`⚠️  Tab ${tabId} stale, disconnecting`);
			conn.ws.close(4002, "Stale connection");
			connectedTabs.delete(tabId);
		} else {
			conn.ws.send(pingMsg);
		}
	}
}, 30000);

// ─── Start Server ──────────────────────────────────────────────────

const _server = Bun.serve<WebSocketData>({
	port: PORT,
	hostname: HOST,
	...(tlsConfig ? { tls: tlsConfig } : {}),
	fetch(req, server) {
		if (req.headers.get("upgrade")?.toLowerCase() === "websocket") {
			const ip = server.requestIP(req)?.address ?? "127.0.0.1";
			const reqUrl = new URL(req.url);
			const authResult = authorizeWs(
				ip,
				reqUrl.pathname + reqUrl.search,
				req.headers,
				authConfig,
			);
			if (!authResult.ok) {
				console.log(`❌ WebSocket rejected: ${authResult.error}`);
				return new Response(authResult.error, { status: 401 });
			}
			server.upgrade(req, {
				data: { ip },
				headers: { "Access-Control-Allow-Private-Network": "true" },
			});
			return undefined;
		}
		return handleRequest(req, server);
	},
	websocket: {
		open: handleWsOpen,
		message: handleWsMessage,
		close: handleWsClose,
	},
});

console.log(`
╔══════════════════════════════════════════════════════════╗
║           HTR NControl Server v0.1.0                 ║
╠══════════════════════════════════════════════════════════╣
║                                                          ║
║  HTTP API:  ${TLS_ENABLED ? "https" : "http"}://${HOST}:${PORT}                      ║
║  WebSocket: ${TLS_ENABLED ? "wss" : "ws"}://${HOST}:${PORT}                         ║
║                                                          ║
║  Auth:                                                  ║
║    IP Whitelist: ${authConfig.enableIpWhitelist ? "✅ Enabled" : "❌ Disabled"}                            ║
║    Bearer Token: ${authConfig.enableBearerToken ? "✅ Required" : "❌ Disabled"}                           ║
║                                                          ║
║  Endpoints:                                             ║
║    GET  /api/health       - Health check                 ║
║    GET  /api/tabs         - List connected tabs          ║
║    GET  /api/tabs/:id     - Get tab info                 ║
║    POST /api/tabs/:id/command - Execute command          ║
║    POST /api/command      - Execute on active tab        ║
║    GET  /api/page         - Get page info                ║
║    GET  /api/screenshot   - Take screenshot              ║
║                                                          ║
╚══════════════════════════════════════════════════════════╝
`);

console.log(
	`💡 Connect the Chrome extension to ${TLS_ENABLED ? "wss" : "ws"}://${HOST}:${PORT}`,
);
console.log(`💡 Send commands via HTTP to http://${HOST}:${PORT}/api/command`);
console.log(``);

// Print example usage
console.log(`📖 Example (curl):`);
console.log(`   # List connected tabs`);
console.log(`   curl http://${HOST}:${PORT}/api/tabs`);
console.log(``);
console.log(`   # Click an element`);
console.log(`   curl -X POST http://${HOST}:${PORT}/api/command \\`);
console.log(`     -H "Content-Type: application/json" \\`);
console.log(
	`     -d '{"command":{"id":"1","action":"click","target":{"selector":"#submit-button"}}}'`,
);
console.log(``);
console.log(`   # Fill a form field`);
console.log(`   curl -X POST http://${HOST}:${PORT}/api/command \\`);
console.log(`     -H "Content-Type: application/json" \\`);
console.log(
	`     -d '{"command":{"id":"2","action":"fill","target":{"name":"email"},"value":"user@example.com"}}'`,
);
console.log(``);
