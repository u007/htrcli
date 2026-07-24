// Firefox-specific MAIN-world dialog-override entry. Same pattern as the
// other Firefox entry shims: import the polyfill first, then the shared
// source. dialogOverride imports dialogPolicy (pure logic, no chrome.*/
// browser.* APIs), but every Firefox entry point imports the polyfill for
// consistency and in case that changes.
import "./browser-polyfill";
import "../src/contentScript/dialogOverride";
