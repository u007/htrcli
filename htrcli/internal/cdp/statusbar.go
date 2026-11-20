package cdp

import (
	"fmt"
	"os"
	"time"
)

// The extension renders its control status line into a shadow-DOM host attached
// to documentElement. htrcli's CDP captures composite the page, so an adopted
// Chrome that already runs the extension can include the line in a screenshot or
// a recording. Hiding the host around a capture keeps --cdp evidence consistent
// with extension-background captures.
//
// Holds are stored on the element, not in this process: a screencast and a
// separate `htrcli screenshot` are different processes, and a naive
// hide/restore pair from the second would restore the bar mid-recording.
// `Runtime.evaluate` is atomic on the page's main thread, so the counters are
// safe across processes.
//
// Two hold kinds, because their lifetimes differ: `shot` (a screenshot) expires
// after 60 s, `record` (a screencast) after 6 h. An expired hold is treated as
// leaked by a killed process and pruned on the next hide of the same kind, so a
// dead process cannot wedge the counter forever. When the last hold ends, the
// inline override is removed and a refresh event asks the content script to
// re-render, leaving visibility to the extension's own state.
//
// Best effort by design: a page that never ran the extension has no host, and a
// browser-internal target cannot evaluate script. A failure is logged, never
// fatal — the capture itself must still happen.
type statusBarHold string

const (
	holdShot   statusBarHold = "shot"
	holdRecord statusBarHold = "record"
)

const (
	shotHoldTTLms   = 60 * 1000
	recordHoldTTLms = 6 * 60 * 60 * 1000
	statusBarHostID = "htrncontrol-control-status"
)

// statusBarCallTimeout bounds one hide/restore round trip so a navigating or
// dead page cannot hang a capture or a recorder stop. Overridable in tests.
var statusBarCallTimeout = 2 * time.Second

// holdAttrs returns the TTL, this kind's counter/timestamp dataset keys, and the
// other kind's counter, which decides whether the display can be released.
func holdAttrs(hold statusBarHold) (ttl int, countKey, sinceKey, otherCountKey string) {
	if hold == holdRecord {
		return recordHoldTTLms, "htrncontrolRecordHolds", "htrncontrolRecordSince", "htrncontrolShotHolds"
	}
	return shotHoldTTLms, "htrncontrolShotHolds", "htrncontrolShotSince", "htrncontrolRecordHolds"
}

func hideScript(hold statusBarHold) string {
	ttl, countKey, sinceKey, _ := holdAttrs(hold)
	return fmt.Sprintf(`(() => {
  const el = document.getElementById("%s");
  if (!el) return false;
  const now = Date.now();
  let count = Number(el.dataset.%s || "0");
  const since = Number(el.dataset.%s || "0");
  if (count > 0 && now - since > %d) count = 0;
  if (count === 0) el.dataset.%s = String(now);
  el.dataset.%s = String(count + 1);
  el.style.display = "none";
  return true;
})()`, statusBarHostID, countKey, sinceKey, ttl, sinceKey, countKey)
}

func restoreScript(hold statusBarHold) string {
	_, countKey, sinceKey, otherCountKey := holdAttrs(hold)
	return fmt.Sprintf(`(() => {
  const el = document.getElementById("%s");
  if (!el) return false;
  let count = Number(el.dataset.%s || "0");
  if (count <= 0) return false;
  count -= 1;
  if (count > 0) {
    el.dataset.%s = String(count);
    return true;
  }
  delete el.dataset.%s;
  delete el.dataset.%s;
  if (Number(el.dataset.%s || "0") > 0) return true;
  el.style.display = "";
  window.dispatchEvent(new Event("htrncontrol:control-status-refresh"));
  return true;
})()`, statusBarHostID, countKey, countKey, countKey, sinceKey, otherCountKey)
}

func evaluateStatusBarScript(s *Session, expression, action string) {
	if err := s.CallDeadline(statusBarCallTimeout, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
	}, nil); err != nil {
		fmt.Fprintf(os.Stderr, "[htrcli] %s: %v\n", action, err)
	}
}

func hideControlStatusBar(s *Session, hold statusBarHold) {
	evaluateStatusBarScript(s, hideScript(hold), "hide control status bar")
}

func restoreControlStatusBar(s *Session, hold statusBarHold) {
	evaluateStatusBarScript(s, restoreScript(hold), "restore control status bar")
}
