package commands

import (
	"errors"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

func BenchmarkParseSelector(b *testing.B) {
	cases := []string{"#submit", "text=Continue", "role=button", "xy=100,200", "@e42", "//div[@class='x']"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseSelector(cases[i%len(cases)])
	}
}

func BenchmarkGlobContains(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		globContains("*/api/users*", "https://example.com/api/users/42?x=1")
	}
}

func TestWaitConditionJS(t *testing.T) {
	waitSelector, waitText, waitURL, waitGone = "", "", "", false
	if _, err := waitConditionJS(); err == nil {
		t.Fatal("expected error with no condition")
	}
	waitText = "hello"
	got, err := waitConditionJS()
	if err != nil || !strContains(got, "hello") {
		t.Fatalf("text cond: %v %s", err, got)
	}
	waitText, waitURL = "", "example.com"
	got, err = waitConditionJS()
	if err != nil || !strContains(got, "example.com") {
		t.Fatalf("url cond: %v %s", err, got)
	}
	waitURL = ""
}

func strContains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestValidateWaitFlags(t *testing.T) {
	cases := []struct {
		name          string
		sel, txt, url string
		gone          bool
		to, iv        int
		wantErr       bool
	}{
		{name: "selector ok", sel: "#x", to: 1000, iv: 250},
		{name: "text ok", txt: "hi", to: 1000, iv: 250},
		{name: "url ok", url: "example.com", to: 1000, iv: 250},
		{name: "gone with selector ok", sel: "#x", gone: true, to: 1000, iv: 250},
		{name: "gone with text", txt: "hi", gone: true, to: 1000, iv: 250, wantErr: true},
		{name: "gone with url", url: "example.com", gone: true, to: 1000, iv: 250, wantErr: true},
		{name: "gone alone", gone: true, to: 1000, iv: 250, wantErr: true},
		{name: "none", to: 1000, iv: 250, wantErr: true},
		{name: "two", sel: "#x", txt: "hi", to: 1000, iv: 250, wantErr: true},
		{name: "three", sel: "#x", txt: "hi", url: "u", to: 1000, iv: 250, wantErr: true},
		{name: "timeout zero", sel: "#x", iv: 250, wantErr: true},
		{name: "timeout neg", sel: "#x", to: -1, iv: 250, wantErr: true},
		{name: "interval zero", sel: "#x", to: 1000, wantErr: true},
		{name: "interval neg", sel: "#x", to: 1000, iv: -5, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			waitSelector, waitText, waitURL = tc.sel, tc.txt, tc.url
			waitGone = tc.gone
			waitTimeout, waitInterval = tc.to, tc.iv
			err := validateWaitFlags()
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v got %v", tc.wantErr, err)
			}
		})
	}
	waitSelector, waitText, waitURL, waitGone = "", "", "", false
	waitTimeout, waitInterval = 10000, 250
}

func TestEvalWaitCheckInversion(t *testing.T) {
	one := []any{map[string]any{"tag": "div"}}
	cases := []struct {
		name string
		data []any
		gone bool
		want bool
	}{
		{"appear, match present", one, false, true},
		{"appear, no match", []any{}, false, false},
		{"gone, match present", one, true, false},
		{"gone, no match", []any{}, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			done, _, err := evalWaitCheck(tc.data, tc.gone)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if done != tc.want {
				t.Fatalf("done = %v, want %v", done, tc.want)
			}
		})
	}
}

// A missing or malformed findAll payload must never read as "no match": in
// gone mode that would report success for a page that never answered.
func TestEvalWaitCheckRejectsNonList(t *testing.T) {
	for _, data := range []any{nil, "x", map[string]any{}, []any{"not-an-object"}} {
		for _, gone := range []bool{false, true} {
			if done, _, err := evalWaitCheck(data, gone); err == nil || done {
				t.Fatalf("data %#v gone=%v: done=%v err=%v, want error and not done", data, gone, done, err)
			}
		}
	}
}

// Only the page's detached-element message is the gone condition. The
// "not known" message (never minted, or navigated) must stay a failure.
func TestIsDetachedRefError(t *testing.T) {
	// Text from src/contentScript/refRegistry.ts (DetachedRefError).
	detached := errors.New("stale ref: @e3 points to an element that is no longer in the document (page re-rendered or navigated)")
	if !isDetachedRefError(detached) {
		t.Fatal("detached-element error must count as gone")
	}
	notKnown := errors.New("stale ref: @e3 is not known on this page (it may have navigated or the ref was minted elsewhere)")
	if isDetachedRefError(notKnown) {
		t.Fatal("not-known ref must not count as gone")
	}
	if isDetachedRefError(errors.New("Execution context was destroyed")) {
		t.Fatal("destroyed context must not count as gone")
	}
}

func TestGoneTimeoutErrorNamesElement(t *testing.T) {
	err := goneTimeoutError(".row", 250, []map[string]any{
		{"tag": "div", "id": "sticky"},
		{"tag": "div"},
	})
	for _, want := range []string{`".row"`, "still present", "250ms", "div#sticky", "2 match(es)"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

func TestGoneUnsettled(t *testing.T) {
	if _, err := goneUnsettled("#x", nil); err == nil {
		t.Fatal("nil pageInfo must be an error, not a gone pass")
	}
	if _, err := goneUnsettled("#x", &api.PageInfo{URL: "about:blank", ReadyState: "complete"}); err == nil {
		t.Fatal("about:blank must be an error, not a gone pass")
	}
	loading, err := goneUnsettled("#x", &api.PageInfo{URL: "https://example.com/", ReadyState: "loading"})
	if err != nil || !loading {
		t.Fatalf("loading document: unsettled=%v err=%v, want unsettled", loading, err)
	}
	loading, err = goneUnsettled("#x", &api.PageInfo{URL: "https://example.com/", ReadyState: "complete"})
	if err != nil || loading {
		t.Fatalf("complete document: unsettled=%v err=%v, want settled", loading, err)
	}
}

// An appear-shaped answer from an old extension must never pass --gone.
func TestRequireGoneAck(t *testing.T) {
	if err := requireGoneAck(map[string]any{"tag": "div", "selector": "#x"}); err == nil {
		t.Fatal("appear answer must be refused")
	}
	if err := requireGoneAck(map[string]any{"gone": true}); err != nil {
		t.Fatalf("gone ack refused: %v", err)
	}
	if err := requireGoneAck(nil); err == nil {
		t.Fatal("nil answer must be refused")
	}
}
