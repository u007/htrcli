package commands

import "testing"

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
	waitSelector, waitText, waitURL = "", "", ""
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
		to, iv        int
		wantErr       bool
	}{
		{"selector ok", "#x", "", "", 1000, 250, false},
		{"text ok", "", "hi", "", 1000, 250, false},
		{"url ok", "", "", "example.com", 1000, 250, false},
		{"none", "", "", "", 1000, 250, true},
		{"two", "#x", "hi", "", 1000, 250, true},
		{"three", "#x", "hi", "u", 1000, 250, true},
		{"timeout zero", "#x", "", "", 0, 250, true},
		{"timeout neg", "#x", "", "", -1, 250, true},
		{"interval zero", "#x", "", "", 1000, 0, true},
		{"interval neg", "#x", "", "", 1000, -5, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			waitSelector, waitText, waitURL = tc.sel, tc.txt, tc.url
			waitTimeout, waitInterval = tc.to, tc.iv
			err := validateWaitFlags()
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v got %v", tc.wantErr, err)
			}
		})
	}
	waitSelector, waitText, waitURL = "", "", ""
	waitTimeout, waitInterval = 10000, 250
}
