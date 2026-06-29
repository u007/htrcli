// Firefox-specific background entry. Imports the polyfill FIRST (so
// `chrome` is defined on `globalThis` before any shared module touches
// it), then applies Firefox-specific shims for Chrome-only APIs
// (`chrome.sidePanel`), then re-exports the shared background module.
// Vite bundles everything into a single `background.js` that the
// manifest references.
import "./browser-polyfill";
import "./firefox-shims";
import "../src/background/index";
