// Firefox-specific MAIN-world console-capture entry. Same pattern as the
// other Firefox entry shims: import the polyfill first (consoleCapture
// itself uses no chrome.*/browser.* APIs today, but every Firefox entry
// point imports this for consistency and in case that changes) then the
// shared source.
import "./browser-polyfill";
import "../src/contentScript/consoleCapture";
