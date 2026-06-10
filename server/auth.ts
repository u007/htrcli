/**
 * Authentication & Authorization for the How-To Recorder Server
 *
 * Supports two modes (both enabled by default):
 *   1. IP Whitelist - Only allows connections from whitelisted IPs
 *   2. Bearer Token - Requires a token in the Authorization header
 *
 * Both can be enabled simultaneously (both must pass).
 * A random token is auto-generated on each server start if not provided.
 */

// ─── Configuration ─────────────────────────────────────────────────

export interface AuthConfig {
	/** Enable IP whitelist checking (default: true) */
	enableIpWhitelist: boolean;
	/** Allowed IPs (default: 127.0.0.1, localhost, ::1) */
	allowedIps: string[];
	/** Enable bearer token checking (default: true) */
	enableBearerToken: boolean;
	/** Required bearer token (if enableBearerToken is true) */
	bearerToken: string;
}

const DEFAULT_ALLOWED_IPS = [
	"127.0.0.1",
	"localhost",
	"::1",
	"::ffff:127.0.0.1",
];

/**
 * Load auth config from environment variables
 */
export function loadAuthConfig(): AuthConfig {
	return {
		enableIpWhitelist: parseBool(process.env.HTR_ENABLE_IP_WHITELIST, true),
		allowedIps: parseList(process.env.HTR_ALLOWED_IPS, DEFAULT_ALLOWED_IPS),
		enableBearerToken: parseBool(process.env.HTR_ENABLE_BEARER_TOKEN, true),
		bearerToken: process.env.HTR_BEARER_TOKEN || generateToken(),
	};
}

/**
 * Check if a request from the given IP and headers is authorized.
 * Returns { ok: true } or { ok: false, error: string }.
 */
export function authorize(
	ip: string,
	headers: Headers,
	config: AuthConfig,
): { ok: true } | { ok: false; error: string } {
	// Normalize IP
	const normalizedIp = normalizeIp(ip);

	// Check IP whitelist
	if (config.enableIpWhitelist) {
		const allowed = config.allowedIps.some((allowedIp) => {
			const normalized = normalizeIp(allowedIp);
			return (
				normalizedIp === normalized ||
				normalizedIp === `::ffff:${normalized}` ||
				`::ffff:${normalizedIp}` === normalized
			);
		});

		if (!allowed) {
			return {
				ok: false,
				error: `IP ${ip} is not in the whitelist. Allowed: ${config.allowedIps.join(", ")}`,
			};
		}
	}

	// Check bearer token
	if (config.enableBearerToken) {
		const authHeader = headers.get("authorization");
		if (!authHeader) {
			return {
				ok: false,
				error: "Missing Authorization header. Expected: Bearer <token>",
			};
		}

		const parts = authHeader.split(" ");
		if (parts.length !== 2 || parts[0] !== "Bearer") {
			return {
				ok: false,
				error: "Invalid Authorization header format. Expected: Bearer <token>",
			};
		}

		if (parts[1] !== config.bearerToken) {
			return {
				ok: false,
				error: "Invalid bearer token",
			};
		}
	}

	return { ok: true };
}

/**
 * Check if a WebSocket upgrade request is authorized
 */
export function authorizeWs(
	ip: string,
	url: string,
	headers: Headers,
	config: AuthConfig,
): { ok: true } | { ok: false; error: string } {
	// For WebSocket, token can be passed as query parameter or header
	let token = headers.get("authorization")?.split(" ")[1];

	// Also check query parameter
	if (!token) {
		try {
			const urlObj = new URL(url, `http://${ip}`);
			token = urlObj.searchParams.get("token") || undefined;
		} catch {
			// Ignore URL parse errors
		}
	}

	// Create modified headers with token
	const modifiedHeaders = new Headers(headers);
	if (token) {
		modifiedHeaders.set("authorization", `Bearer ${token}`);
	}

	return authorize(ip, modifiedHeaders, config);
}

// ─── Helpers ───────────────────────────────────────────────────────

function normalizeIp(ip: string): string {
	// Remove IPv6 prefix
	if (ip.startsWith("::ffff:")) {
		return ip.slice(7);
	}
	// Map localhost to 127.0.0.1
	if (ip === "localhost" || ip === "::1") {
		return "127.0.0.1";
	}
	return ip;
}

function parseBool(value: string | undefined, defaultValue: boolean): boolean {
	if (value === undefined) return defaultValue;
	return value === "true" || value === "1";
}

function parseList(
	value: string | undefined,
	defaultValue: string[],
): string[] {
	if (!value) return defaultValue;
	return value
		.split(",")
		.map((s) => s.trim())
		.filter(Boolean);
}

/**
 * Generate a random 32-character hex token
 */
function generateToken(): string {
	const bytes = new Uint8Array(16);
	crypto.getRandomValues(bytes);
	return Array.from(bytes)
		.map((b) => b.toString(16).padStart(2, "0"))
		.join("");
}
