/**
 * Test preload: registers happy-dom globals so `bun test` runs with a real
 * `document`/`window` available. This lets content-script tests create and
 * assert on DOM elements without a browser.
 *
 * Wired into `bunfig.toml` under `[test] preload`.
 *
 * NOTE: `bun test` executes each test file in its own realm and re-registers a
 * fresh happy-dom `Window` per realm. Global mutations made here do NOT survive
 * into those per-file realms, so DOM-backed tests must also import
 * `./test/domSetup` (which installs the viewport/rect mocks in the test file's
 * own realm). See that module for details.
 */
import { GlobalRegistrator } from "@happy-dom/global-registrator";

GlobalRegistrator.register();
