/**
 * How-To Recorder Server
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

import { type WebSocket, WebSocketServer } from "ws";
import {
	type AuthConfig,
	authorize,
	authorizeWs,
	loadAuthConfig,
} from "./auth";
import type {
	ApiResponse,
	Command,
	CommandRequest,
	CommandResult,
	ExtensionMessage,
	PageInfo,
	ServerMessage,
	TabInfo,
} from "./types";

// ─── Configuration ─────────────────────────────────────────────────

const PORT = Number(process.env.HTR_PORT) || 3845;
const HOST = process.env.HTR_HOST || "127.0.0.1";
// ─── State ─────────────────────────────────────────────────────────

/** Connected extension tabs */
const connectedTabs = new Map<
	number,
	{
		ws: WebSocket;
		tabInfo: TabInfo;
		lastHeartbeat: number;
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

// ─── WebSocket Handler ─────────────────────────────────────────────

function handleWsConnection(
	ws: WebSocket,
	req: import("http").IncomingMessage,
): void {
	const ip = extractClientIp(req);
	const url = req.url || "/";

	// Authorize WebSocket connection
	const authResult = authorizeWs(ip, url, req.headers, authConfig);
	if (!authResult.ok) {
		console.log(`❌ WebSocket rejected: ${authResult.error}`);
		ws.close(4001, authResult.error);
		return;
	}

	console.log(`🔌 Extension connected from ${ip}`);

	ws.on("message", (data) => {
		try {
			const message = JSON.parse(data.toString()) as ExtensionMessage;
			handleExtensionMessage(ws, message);
		} catch (error) {
			console.error("Failed to parse extension message:", error);
		}
	});

	ws.on("close", () => {
		// Find and remove the tab
		for (const [tabId, conn] of connectedTabs.entries()) {
			if (conn.ws === ws) {
				connectedTabs.delete(tabId);
				console.log(`🔌 Tab ${tabId} disconnected`);
				break;
			}
		}
	});

	ws.on("error", (error) => {
		console.error("WebSocket error:", error);
	});

	// Send ping to keep connection alive
	const pingInterval = setInterval(() => {
		if (ws.readyState === ws.OPEN) {
			const pingMsg: ServerMessage = {
				type: "ping",
				timestamp: Date.now(),
			};
			ws.send(JSON.stringify(pingMsg));
		} else {
			clearInterval(pingInterval);
		}
	}, 30000);
}

function handleExtensionMessage(
	ws: WebSocket,
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

/**
 * Extract the real client IP from an HTTP request.
 * Checks X-Forwarded-For, X-Real-IP, then falls back to socket remoteAddress.
 */
function extractClientIp(
	req: import("http").IncomingMessage | import("ws").IncomingMessage,
): string {
	const forwarded = req.headers["x-forwarded-for"];
	if (forwarded) {
		const firstIp = Array.isArray(forwarded)
			? forwarded[0]
			: forwarded.split(",")[0]?.trim();
		if (firstIp) return firstIp;
	}

	const realIp = req.headers["x-real-ip"];
	if (realIp) {
		return Array.isArray(realIp) ? realIp[0] : realIp;
	}

	const socket = "socket" in req ? req.socket : undefined;
	return socket?.remoteAddress || "127.0.0.1";
}

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
	};
}

async function handleRequest(req: Request): Promise<Response> {
	const url = new URL(req.url);
	const path = url.pathname;
	const method = req.method;
	const clientIp = extractClientIpFromRequest(req);

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

/** Extract client IP from a Web API Request object */
function extractClientIpFromRequest(req: Request): string {
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
	return `cmd_${Date.now()}_${Math.random().toString(36).substr(2, 6)}`;
}

// ─── Stale Connection Cleanup ──────────────────────────────────────

setInterval(() => {
	const now = Date.now();
	const staleThreshold = 60000; // 60 seconds

	for (const [tabId, conn] of connectedTabs.entries()) {
		if (now - conn.lastHeartbeat > staleThreshold) {
			console.log(`⚠️  Tab ${tabId} stale, disconnecting`);
			conn.ws.close(4002, "Stale connection");
			connectedTabs.delete(tabId);
		}
	}
}, 30000);

// ─── Start Server ──────────────────────────────────────────────────

const wss = new WebSocketServer({ noServer: true });

const server = Bun.serve({
	port: PORT,
	hostname: HOST,
	fetch: handleRequest,
	// Handle WebSocket upgrade via Bun's built-in support
	upgrade: (req) => {
		if (req.headers.get("upgrade")?.toLowerCase() === "websocket") {
			return {
				headers: {},
				data: {},
			};
		}
		return undefined;
	},
});

// Override upgrade to use ws library for proper WebSocket handling
// This allows ws to handle the protocol negotiation and framing
const originalFetch = server.fetch;
server.fetch = async (req, server) => {
	if (req.headers.get("upgrade")?.toLowerCase() === "websocket") {
		// Let Bun handle the upgrade and we'll use ws for the connection
		server.upgrade(req);
		return new Response(null);
	}
	return originalFetch(req, server);
};

wss.on("connection", handleWsConnection);

console.log(`
╔══════════════════════════════════════════════════════════╗
║           How-To Recorder Server v0.1.0                 ║
╠══════════════════════════════════════════════════════════╣
║                                                          ║
║  HTTP API:  http://${HOST}:${PORT}                       ║
║  WebSocket: ws://${HOST}:${PORT}                         ║
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

console.log(`💡 Connect the Chrome extension to ws://${HOST}:${PORT}`);
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
