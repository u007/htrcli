import { defineManifest } from "@crxjs/vite-plugin";
import packageData from "../package.json";

const isDev = process.env.NODE_ENV === "development";

export default defineManifest({
	name: `${packageData.displayName || packageData.name}${isDev ? ` ➡️ Dev` : ""}`,
	description: packageData.description,
	version: packageData.version,
	manifest_version: 3,
	icons: {
		16: "img/logo-16.png",
		32: "img/logo-32.png",
		48: "img/logo-48.png",
		128: "img/logo-128.png",
	},
	action: {
		default_popup: "popup.html",
		default_icon: "img/logo-48.png",
	},
	options_page: "options.html",
	background: {
		service_worker: "src/background/index.ts",
		type: "module",
	},
	content_scripts: [
		{
			matches: ["http://*/*", "https://*/*"],
			js: ["src/contentScript/index.ts"],
		},
		{
			matches: ["http://*/*", "https://*/*"],
			js: ["src/contentScript/consoleCapture.ts"],
			world: "MAIN",
			run_at: "document_start",
		},
		{
			matches: ["http://*/*", "https://*/*"],
			js: ["src/contentScript/dialogOverride.ts"],
			world: "MAIN",
			run_at: "document_start",
		},
	],
	side_panel: {
		default_path: "sidepanel.html",
	},
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
		"activeTab", // Access current tab for screenshots
		"tabs", // Track tab navigation & new tabs
		"contextMenus", // Right-click annotation (optional)
		"downloads", // Export files
		"storage", // Store session metadata
		"scripting", // Inject content scripts into new tabs
		"sidePanel", // Main UI
		"nativeMessaging", // Connect to htrcli native host
		"debugger", // CDP Page.printToPDF for headless PDF capture
		"webRequest", // Firefox passive network capture (observation only)
	],
	host_permissions: ["<all_urls>"], // Content script injection on any page
});
