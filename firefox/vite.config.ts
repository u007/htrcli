// Firefox build entry — plain Vite (no `@crxjs/vite-plugin`, which
// doesn't understand Firefox's `sidebar_action`).
//
// We re-use the shared source under `src/` via thin Firefox entry
// shims (see `./src/*-entry.ts`) that import the
// `webextension-polyfill` before delegating to the shared module, so
// the `chrome.*` calls in the shared source resolve to Firefox's
// `browser` API transparently.
//
// The Vite root is set to the project root so that the entry shims
// can resolve `../src/...` (which lives at the project root). The
// Firefox-specific HTMLs live in `firefox/` and are referenced via
// `rollupOptions.input` using absolute paths.
//
// A custom Vite plugin (`buildFirefoxManifest`) runs after the
// production build and emits a `manifest.json` in `firefox/build/`
// that matches Firefox's MV3 schema and references the built
// bundles/HTML files.

import { existsSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";

// Vite extracts bundled configs into a temp directory, so the bundled
// `import.meta.url` no longer points to the original source file.
// The most reliable way to locate the project root is the current
// working directory (the project root when Vite is invoked from
// package.json scripts).
const projectRoot = process.cwd();
const fallbackRoot = dirname(fileURLToPath(import.meta.url));
const configRoot = existsSync(resolve(projectRoot, "firefox/vite.config.ts"))
	? projectRoot
	: fallbackRoot;

// The manifest object is built from a plain JSON-compatible JS
// literal — no `defineManifest` typing constraints, so we can declare
// Firefox-only keys like `sidebar_action` directly. We read
// `package.json` from disk because the Vite config is bundled to ESM
// (no `require`).
interface PackageData {
	displayName?: string;
	name: string;
	version: string;
	description: string;
}
const packageData = JSON.parse(
	readFileSync(resolve(configRoot, "package.json"), "utf8"),
) as PackageData;

const isDev = process.env.NODE_ENV === "development";

/**
 * Vite plugin: after a production build, write a `manifest.json` into
 * `firefox/build/` that describes the Firefox extension. Background
 * and content-script entry points are emitted with stable filenames
 * (`background.js`, `content.js`) by the `entryFileNames` callback in
 * `rollupOptions.output`, so the manifest can reference them by name.
 */
function buildFirefoxManifest(): Plugin {
	return {
		name: "build-firefox-manifest",
		apply: "build",
		generateBundle() {
			const manifest = {
				name: `${packageData.displayName || packageData.name}${isDev ? " ➡️ Dev" : ""}`,
				description: packageData.description,
				version: packageData.version,
				manifest_version: 3,
				icons: {
					16: "img/logo-16.png",
					32: "img/logo-32.png",
					48: "img/logo-48.png",
					128: "img/logo-128.png",
				},
				// `sidebar_action` mirrors Chrome's `side_panel`
				// (same URL, opens a persistent sidebar docked to the
				// browser window).
				sidebar_action: {
					default_title: packageData.displayName || packageData.name,
					default_panel: "firefox/sidepanel.html",
					default_icon: "img/logo-48.png",
				},
				// `action` is the MV3 toolbar button.
				action: {
					default_title: packageData.displayName || packageData.name,
					default_icon: "img/logo-48.png",
				},
				options_ui: {
					page: "firefox/options.html",
					open_in_tab: true,
				},
				background: {
					// Firefox MV3 still prefers `scripts` over
					// `service_worker` (the latter is a Chrome-ism
					// that landed in Firefox 121+; declaring both
					// gives a single manifest that loads in every
					// supported Firefox version and passes
					// `addons-linter`).
					scripts: ["background.js"],
					service_worker: "background.js",
					type: "module",
				},
				content_scripts: [
					{
						matches: ["http://*/*", "https://*/*"],
						js: ["content.js"],
						run_at: "document_start",
						all_frames: false,
					},
					{
						matches: ["http://*/*", "https://*/*"],
						js: ["consoleCapture.js"],
						world: "MAIN",
						run_at: "document_start",
						all_frames: false,
					},
					{
						matches: ["http://*/*", "https://*/*"],
						js: ["dialogOverride.js"],
						world: "MAIN",
						run_at: "document_start",
						all_frames: false,
					},
				],
				web_accessible_resources: [
					{
						resources: [
							"img/logo-16.png",
							"img/logo-32.png",
							"img/logo-48.png",
							"img/logo-128.png",
						],
						matches: ["<all_urls>"],
					},
				],
				permissions: [
					"activeTab",
					"tabs",
					"contextMenus",
					"downloads",
					"storage",
					"scripting",
					"nativeMessaging",
					"webRequest",
					"webRequestBlocking",
				],
				host_permissions: ["<all_urls>"],
				browser_specific_settings: {
					gecko: {
						id: "htrncontrol@mercstudio.com",
						// 128+ is required for content_scripts world:
						// "MAIN" (used by the console-capture script) —
						// this supersedes the prior 112.0 floor, which
						// only covered `background.type` and the
						// data-collection-consent UX declared below.
						strict_min_version: "128.0",
						// Required by addons-linter for new Firefox
						// extension submissions. We don't collect
						// any data ourselves; the optional remote
						// control server is opt-in by the user.
						data_collection_permissions: {
							required: ["none"],
						},
					},
				},
			};

			// Emit manifest.json into the build output.
			this.emitFile({
				type: "asset",
				fileName: "manifest.json",
				source: JSON.stringify(manifest, null, 2),
			});
		},
	};
}

export default defineConfig(() => {
	return {
		// We use the project root as Vite root so that the Firefox
		// entry shims can resolve `../src/...` against the shared
		// `src/` tree. Public assets are then served from
		// `firefox/public/`, and the HTML entries point at the
		// Firefox-specific files in `firefox/`.
		root: configRoot,
		publicDir: resolve(configRoot, "firefox/public"),
		// Emit relative asset paths in HTML/JS so they resolve
		// correctly when the extension is loaded from a
		// `moz-extension://` URL (where the root is the extension
		// package, not the host page). This is the same default
		// Firefox uses for unpacked extensions.
		base: "./",
		resolve: {
			// The Firefox entry shims live in `firefox/src/` and reach
			// into the shared source via `../src/...`. Vite's bundler
			// treats that `..` as "outside the root" and refuses to
			// resolve it. Aliasing `../src` to an absolute path
			// inside the root fixes that without changing the shim
			// sources.
			alias: {
				"../src": resolve(configRoot, "src"),
			},
		},
		build: {
			emptyOutDir: true,
			outDir: resolve(configRoot, "firefox/build"),
			// HTML files end up in `firefox/build/firefox/` and the
			// top-level JS bundles in `firefox/build/`. The manifest
			// paths below reflect that layout.
			rollupOptions: {
				input: {
					// HTML pages — these are loaded by the user.
					// Listed as relative paths so Vite preserves
					// the directory structure in the output.
					sidepanel: "firefox/sidepanel.html",
					popup: "firefox/popup.html",
					options: "firefox/options.html",
					// Background service worker and content scripts —
					// not loaded by HTML, but referenced by the
					// emitted `manifest.json`. Input keys are
					// deliberately *without* the `.js` extension; the
					// `entryFileNames` callback below adds it back.
					background: "firefox/src/background-entry.ts",
					content: "firefox/src/contentScript-entry.ts",
					consoleCapture: "firefox/src/consoleCapture-entry.ts",
					dialogOverride: "firefox/src/dialogOverride-entry.ts",
				},
				output: {
					chunkFileNames: "assets/chunk-[hash].js",
					// Always emit a `.js` extension on entry chunks.
					// The HTML inputs (which Vite also processes)
					// emit their own `.html` files through the
					// Vite HTML plugin and don't go through this
					// callback, so we don't need a special case for
					// them.
					entryFileNames: "[name].js",
				},
			},
		},
		plugins: [react(), buildFirefoxManifest()],
		legacy: {
			skipWebSocketTokenCheck: true,
		},
	};
});
