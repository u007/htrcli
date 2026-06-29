// Firefox-specific sidebar entry. Same polyfill-first pattern so the
// shared React components (which call `chrome.runtime.sendMessage`,
// `chrome.storage.local`, etc.) work unchanged in Firefox.
//
// The resulting bundle is loaded by `sidepanel.html` (the same HTML
// used by the Chrome build — its `<script>` tag points at this entry).
import "./browser-polyfill";
import "../src/sidepanel/index.tsx";
