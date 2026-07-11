/**
 * Load environment variables from the repo-root `.env` files.
 *
 * The server is started with its cwd set to `server/` (via
 * `bun run server` -> `cd server && bun run start`), so Bun's automatic
 * `.env` loading only looks inside `server/` and never finds the
 * repo-root `.env`. The auth/config code also reads `process.env` directly
 * and never parses `.env` files on its own. This module resolves
 * `../.env` and `../.env.local` relative to this file (cwd independent) so
 * the configured `HTR_BEARER_TOKEN`, `HTR_PORT`, etc. are picked up
 * regardless of how the server is launched.
 *
 * Precedence (lowest -> highest):
 *   1. repo-root `.env`
 *   2. repo-root `.env.local`   (local overrides, mirrors Bun/Dotenv convention)
 *   3. variables already present in `process.env` (explicit env beats files)
 *
 * Must be called before any top-level config is read (e.g. `loadAuthConfig()`).
 */
import { readFileSync, existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = resolve(__dirname, "..");

function parseEnvFile(fileName: string): Record<string, string> {
	const full = resolve(REPO_ROOT, fileName);
	const out: Record<string, string> = {};
	if (!existsSync(full)) return out;

	const text = readFileSync(full, "utf8");
	for (const raw of text.split(/\r?\n/)) {
		const line = raw.trim();
		if (!line || line.startsWith("#")) continue;
		const eq = line.indexOf("=");
		if (eq === -1) continue;

		const key = line.slice(0, eq).trim();
		let value = line.slice(eq + 1).trim();
		// Strip surrounding quotes so `KEY="value"` and `KEY=value` behave the same.
		if (
			(value.startsWith('"') && value.endsWith('"')) ||
			(value.startsWith("'") && value.endsWith("'"))
		) {
			value = value.slice(1, -1);
		}
		out[key] = value;
	}
	return out;
}

export function loadEnv(): void {
	// `.env` first, `.env.local` second so local overrides win.
	for (const file of [".env", ".env.local"]) {
		const vars = parseEnvFile(file);
		for (const [key, value] of Object.entries(vars)) {
			if (process.env[key] === undefined) {
				process.env[key] = value;
			}
		}
	}
}
