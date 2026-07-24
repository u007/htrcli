package commands

import (
	"testing"
)

func TestParseSelector_CSS(t *testing.T) {
	s := parseSelector("#submit-btn")
	if s.Selector != "#submit-btn" {
		t.Errorf("expected Selector '#submit-btn', got %q", s.Selector)
	}
}

func TestParseSelector_Name(t *testing.T) {
	s := parseSelector("name=email")
	if s.Name != "email" {
		t.Errorf("expected Name 'email', got %q", s.Name)
	}
}

func TestParseSelector_Role(t *testing.T) {
	s := parseSelector("role=button")
	if s.Role != "button" {
		t.Errorf("expected Role 'button', got %q", s.Role)
	}
}

func TestParseSelector_Text(t *testing.T) {
	s := parseSelector("text=Submit")
	if s.Text != "Submit" {
		t.Errorf("expected Text 'Submit', got %q", s.Text)
	}
}

func TestParseSelector_Label(t *testing.T) {
	s := parseSelector("label=Email address")
	if s.Label != "Email address" {
		t.Errorf("expected Label 'Email address', got %q", s.Label)
	}
}

func TestParseSelector_Placeholder(t *testing.T) {
	s := parseSelector("placeholder=Enter email")
	if s.Placeholder != "Enter email" {
		t.Errorf("expected Placeholder 'Enter email', got %q", s.Placeholder)
	}
}

func TestParseSelector_ID(t *testing.T) {
	s := parseSelector("id=login-form")
	if s.ID != "login-form" {
		t.Errorf("expected ID 'login-form', got %q", s.ID)
	}
}

func TestParseSelector_XPath(t *testing.T) {
	s := parseSelector("xpath=//button[1]")
	if s.XPath != "//button[1]" {
		t.Errorf("expected XPath '//button[1]', got %q", s.XPath)
	}
}

func TestParseSelector_DefaultCSS(t *testing.T) {
	s := parseSelector(".login-form input[type=email]")
	if s.Selector != ".login-form input[type=email]" {
		t.Errorf("expected Selector '.login-form input[type=email]', got %q", s.Selector)
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		seconds  float64
		expected string
	}{
		{0, "0s"},
		{30, "30s"},
		{60, "1m 0s"},
		{90, "1m 30s"},
		{3600, "1h 0m 0s"},
		{3661, "1h 1m 1s"},
		{86400, "24h 0m 0s"},
	}

	for _, tt := range tests {
		result := formatUptime(tt.seconds)
		if result != tt.expected {
			t.Errorf("formatUptime(%v) = %q, expected %q", tt.seconds, result, tt.expected)
		}
	}
}

func TestParseSelector_Ref(t *testing.T) {
	s := parseSelector("@e7")
	if s.Ref != "@e7" {
		t.Errorf("expected Ref '@e7', got %q (selector=%q)", s.Ref, s.Selector)
	}
	if s.Selector != "" {
		t.Errorf("ref arg must not populate Selector, got %q", s.Selector)
	}
}

func TestParseSelector_RefLeavesRealSelectorsAlone(t *testing.T) {
	// An email like "@" mid-string is not a ref; only a leading @ is.
	s := parseSelector("input[name=email]")
	if s.Ref != "" {
		t.Errorf("expected no Ref for a CSS selector, got %q", s.Ref)
	}
}
