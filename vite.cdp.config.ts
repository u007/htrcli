import { resolve } from "node:path";
import { defineConfig } from "vite";

// Builds the htrcli CDP DOM bundle: a single self-contained IIFE with no
// chrome.* requirements (commandExecutor guards all chrome access), written
// directly into the Go embed directory.
export default defineConfig({
	// The extension's public/ assets have no place in the Go embed directory.
	publicDir: false,
	build: {
		outDir: "htrcli/internal/cdp/bundle",
		emptyOutDir: false,
		minify: false,
		lib: {
			entry: resolve(__dirname, "src/cdpBundle/index.ts"),
			formats: ["iife"],
			name: "__htrcliDomBundle",
			fileName: () => "htrcli-dom.js",
		},
	},
});
