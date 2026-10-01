package api

import (
	"encoding/json"
	"testing"
)

// The extension puts `browser` in the recordingList payload and the CLI shows it
// as a column, which is how a user learns that a --browser hint silently fell
// back to a different profile.
//
// This test guards the narrow gap that is easiest to ship broken: Go's decoder
// drops an unknown JSON key without error, so a missing struct field does not
// fail loudly — the column just renders empty forever.
func TestRecordingListDataDecodesBrowser(t *testing.T) {
	raw := []byte(`{
		"sessions": [],
		"total": 0,
		"limit": 50,
		"offset": 0,
		"browser": "firefox"
	}`)

	var data RecordingListData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Browser != "firefox" {
		t.Errorf("Browser = %q, want firefox; if this is empty the field was never added "+
			"to RecordingListData and the CLI column will always be blank", data.Browser)
	}
}

// An older extension does not send the key at all. That must decode cleanly to
// "" rather than erroring, so an out-of-date extension still works.
func TestRecordingListDataToleratesMissingBrowser(t *testing.T) {
	var data RecordingListData
	if err := json.Unmarshal([]byte(`{"sessions":[],"total":0,"limit":50,"offset":0}`), &data); err != nil {
		t.Fatalf("a payload with no browser key must still decode, got %v", err)
	}
	if data.Browser != "" {
		t.Errorf("Browser = %q, want empty when the key is absent", data.Browser)
	}
}
