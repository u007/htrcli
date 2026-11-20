/**
 * Guard against a stale or hand-edited htrcli CDP DOM bundle.
 *
 * `htrcli/internal/cdp/bundle/htrcli-dom.js` is generated from
 * `src/cdpBundle/index.ts`, committed, and embedded into the Go binary with
 * `//go:embed`. Editing it by hand, or changing `src/` without rebuilding,
 * makes the `--cdp` transport silently drift from the extension.
 *
 * This rebuilds into a temp directory and compares the bytes. Run it whenever
 * `src/contentScript` or `src/utils` changes, and in CI.
 */
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const root = process.cwd();
const bundlePath = join(root, "htrcli/internal/cdp/bundle/htrcli-dom.js");
const outDir = mkdtempSync(join(tmpdir(), "htrcli-cdp-bundle-"));

try {
	execFileSync(
		"bunx",
		[
			"vite",
			"build",
			"--config",
			"vite.cdp.config.ts",
			"--outDir",
			outDir,
			"--emptyOutDir",
		],
		{ cwd: root, stdio: "pipe" },
	);

	const built = readFileSync(join(outDir, "htrcli-dom.js"), "utf8");
	const committed = readFileSync(bundlePath, "utf8");

	if (built !== committed) {
		console.error(
			"[HTR NControl] htrcli-dom.js is stale or hand-edited. Run `bun run cdp-bundle:build` and commit the result.",
		);
		process.exit(1);
	}

	console.log("htrcli-dom.js matches a fresh build.");
} finally {
	rmSync(outDir, { recursive: true, force: true });
}
