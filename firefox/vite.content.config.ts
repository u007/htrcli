// Second build pass for the Firefox CONTENT SCRIPT only.
//
// Content scripts are executed as classic scripts — unlike the background,
// they cannot be ES modules. The main Firefox build (vite.config.ts) uses
// code-splitting, which emits `content.js` with top-level `import`
// statements; Firefox then fails every page load with "import declarations
// may only appear at top level of a module" and the content script never
// runs (no tab ever connects). This pass rebuilds the same entry as a
// self-contained IIFE and overwrites `firefox/build/content.js`.
//
// Run AFTER the main build (see the `firefox:build` script): this config
// sets `emptyOutDir: false` so it only replaces content.js.

import { existsSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

const projectRoot = process.cwd();
const fallbackRoot = dirname(fileURLToPath(import.meta.url));
const configRoot = existsSync(resolve(projectRoot, "firefox/vite.config.ts"))
	? projectRoot
	: resolve(fallbackRoot, "..");

export default defineConfig({
	root: configRoot,
	// No public assets in this pass — the main build already copied them.
	publicDir: false,
	base: "./",
	resolve: {
		alias: {
			"../src": resolve(configRoot, "src"),
		},
	},
	build: {
		emptyOutDir: false,
		outDir: resolve(configRoot, "firefox/build"),
		rollupOptions: {
			input: resolve(configRoot, "firefox/src/contentScript-entry.ts"),
			output: {
				// IIFE = classic script, everything inlined; rollup only
				// supports it for a single entry, which is why this is a
				// separate pass instead of another input in the main config.
				format: "iife",
				entryFileNames: "content.js",
				inlineDynamicImports: true,
			},
		},
	},
});
