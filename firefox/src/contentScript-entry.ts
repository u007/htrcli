// Firefox-specific content-script entry. Same pattern as the background
// entry: import the polyfill first so `chrome.*` calls in the shared
// source resolve to Firefox's `browser` API.
import "./browser-polyfill";
import "../src/contentScript/index";
