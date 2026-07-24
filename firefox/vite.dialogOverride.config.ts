// Fourth build pass for the Firefox MAIN-world DIALOG-OVERRIDE content
// script only.
//
// Same problem as vite.content.config.ts / vite.consoleCapture.config.ts:
// content scripts execute as classic scripts, not ES modules, so the main
// build's code-split `dialogOverride.js` (with top-level `import`
// statements) fails to load in Firefox with "import declarations may only
// appear at top level of a module" and the override never runs. This pass
// rebuilds the same entry as a self-contained IIFE and overwrites
// `firefox/build/dialogOverride.js`.
//
// Run AFTER the main build and the other content-script passes (see the
// `firefox:build` script): this config sets `emptyOutDir: false` so it only
// replaces dialogOverride.js.

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
			input: resolve(configRoot, "firefox/src/dialogOverride-entry.ts"),
			output: {
				format: "iife",
				entryFileNames: "dialogOverride.js",
				inlineDynamicImports: true,
			},
		},
	},
});
