import { beforeEach, describe, expect, it } from "bun:test";
import { authorize, authorizeWs, loadAuthConfig } from "./auth";

describe("Auth", () => {
	describe("loadAuthConfig", () => {
		const originalEnv = process.env;

		beforeEach(() => {
			process.env = { ...originalEnv };
			delete process.env.HTR_ENABLE_IP_WHITELIST;
			delete process.env.HTR_ALLOWED_IPS;
			delete process.env.HTR_ENABLE_BEARER_TOKEN;
			delete process.env.HTR_BEARER_TOKEN;
		});

		it("should return default config when no env vars set", () => {
			const config = loadAuthConfig();
			expect(config.enableIpWhitelist).toBe(true);
			expect(config.enableBearerToken).toBe(true);
			expect(config.allowedIps).toContain("127.0.0.1");
			expect(config.allowedIps).toContain("localhost");
			expect(config.allowedIps).toContain("::1");
			// Auto-generated token should be a 32-char hex string
			expect(config.bearerToken.length).toBe(32);
			expect(/^[0-9a-f]+$/.test(config.bearerToken)).toBe(true);
		});

		it("should use custom bearer token from env", () => {
			process.env.HTR_BEARER_TOKEN = "my-secret-token";
			const config = loadAuthConfig();
			expect(config.bearerToken).toBe("my-secret-token");
		});

		it("should disable IP whitelist from env", () => {
			process.env.HTR_ENABLE_IP_WHITELIST = "false";
			const config = loadAuthConfig();
			expect(config.enableIpWhitelist).toBe(false);
		});

		it("should disable bearer token from env", () => {
			process.env.HTR_ENABLE_BEARER_TOKEN = "false";
			const config = loadAuthConfig();
			expect(config.enableBearerToken).toBe(false);
		});

		it("should parse custom allowed IPs from env", () => {
			process.env.HTR_ALLOWED_IPS = "192.168.1.100,10.0.0.1";
			const config = loadAuthConfig();
			expect(config.allowedIps).toEqual(["192.168.1.100", "10.0.0.1"]);
		});
	});

	describe("authorize", () => {
		const defaultConfig = {
			enableIpWhitelist: true,
			allowedIps: ["127.0.0.1", "localhost", "::1"],
			enableBearerToken: true,
			bearerToken: "test-token-123",
		};

		function makeHeaders(auth?: string): Headers {
			const h = new Headers();
			if (auth) h.set("authorization", auth);
			return h;
		}

		it("should allow localhost with valid token", () => {
			const result = authorize(
				"127.0.0.1",
				makeHeaders("Bearer test-token-123"),
				defaultConfig,
			);
			expect(result.ok).toBe(true);
		});

		it("should reject wrong IP", () => {
			const result = authorize(
				"192.168.1.100",
				makeHeaders("Bearer test-token-123"),
				defaultConfig,
			);
			expect(result.ok).toBe(false);
			if (!result.ok) {
				expect(result.error).toContain("not in the whitelist");
			}
		});

		it("should reject missing bearer token", () => {
			const result = authorize("127.0.0.1", makeHeaders(), defaultConfig);
			expect(result.ok).toBe(false);
			if (!result.ok) {
				expect(result.error).toContain("Missing Authorization header");
			}
		});

		it("should reject wrong bearer token", () => {
			const result = authorize(
				"127.0.0.1",
				makeHeaders("Bearer wrong-token"),
				defaultConfig,
			);
			expect(result.ok).toBe(false);
			if (!result.ok) {
				expect(result.error).toContain("Invalid bearer token");
			}
		});

		it("should skip IP check when disabled", () => {
			const config = { ...defaultConfig, enableIpWhitelist: false };
			const result = authorize(
				"192.168.1.100",
				makeHeaders("Bearer test-token-123"),
				config,
			);
			expect(result.ok).toBe(true);
		});

		it("should skip bearer check when disabled", () => {
			const config = { ...defaultConfig, enableBearerToken: false };
			const result = authorize("127.0.0.1", makeHeaders(), config);
			expect(result.ok).toBe(true);
		});

		it("should handle IPv6 addresses", () => {
			const result = authorize(
				"::1",
				makeHeaders("Bearer test-token-123"),
				defaultConfig,
			);
			expect(result.ok).toBe(true);
		});

		it("should handle IPv4-mapped IPv6 addresses", () => {
			const result = authorize(
				"::ffff:127.0.0.1",
				makeHeaders("Bearer test-token-123"),
				defaultConfig,
			);
			expect(result.ok).toBe(true);
		});

		it("should reject malformed Authorization header", () => {
			const result = authorize(
				"127.0.0.1",
				makeHeaders("Token test-token-123"),
				defaultConfig,
			);
			expect(result.ok).toBe(false);
			if (!result.ok) {
				expect(result.error).toContain("Invalid Authorization header format");
			}
		});
	});

	describe("authorizeWs", () => {
		const defaultConfig = {
			enableIpWhitelist: true,
			allowedIps: ["127.0.0.1", "localhost", "::1"],
			enableBearerToken: true,
			bearerToken: "ws-token-456",
		};

		it("should allow WS from localhost with token in URL", () => {
			const result = authorizeWs(
				"127.0.0.1",
				"/?token=ws-token-456",
				new Headers(),
				defaultConfig,
			);
			expect(result.ok).toBe(true);
		});

		it("should reject WS from wrong IP", () => {
			const result = authorizeWs(
				"192.168.1.100",
				"/?token=ws-token-456",
				new Headers(),
				defaultConfig,
			);
			expect(result.ok).toBe(false);
		});

		it("should reject WS with wrong token", () => {
			const result = authorizeWs(
				"127.0.0.1",
				"/?token=wrong",
				new Headers(),
				defaultConfig,
			);
			expect(result.ok).toBe(false);
		});

		it("should skip checks when both disabled", () => {
			const config = {
				enableIpWhitelist: false,
				enableBearerToken: false,
				allowedIps: [],
				bearerToken: "",
			};
			const result = authorizeWs("10.0.0.1", "/", new Headers(), config);
			expect(result.ok).toBe(true);
		});
	});
});
