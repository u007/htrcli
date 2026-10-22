package cdp

import (
	"strings"
	"testing"
)

func TestTrimTrailingSemicolon(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"({a: 1});", "({a: 1})"},
		{"({a: 1});\n", "({a: 1})"},
		{"({a: 1});\n\n", "({a: 1})"},
		{"({a: 1})", "({a: 1})"},
		{"({a: 1})   ", "({a: 1})"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := trimTrailingSemicolon(tc.in); got != tc.want {
			t.Errorf("trimTrailingSemicolon(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestFormKitInstallIsSyntacticallyValid guards the form-kit install
// expression. The source is wrapped in parentheses, so a trailing semicolon
// makes the whole install a syntax error ("Unexpected token ';'") and every
// `pick` / `fill-form` over CDP fails with a misleading "installing form kit"
// error instead of doing its job.
//
// The check looks at the characters immediately BEFORE the wrapper's closing
// parenthesis: the broken form ends with "});\n\n)" — the semicolon is
// inside the parens, so a naive "ends with )" assertion passes even when the
// expression is invalid.
func TestFormKitInstallIsSyntacticallyValid(t *testing.T) {
	install := formKitSpec.install()
	if !strings.HasPrefix(install, "window.__htrcliFormKit = (") {
		t.Fatalf("install should assign the kit to a global, got %q", install[:40])
	}
	trimmed := strings.TrimSpace(install)
	if !strings.HasSuffix(trimmed, ")") {
		t.Fatalf("install should end with a closing parenthesis, got %q", trimmed[len(trimmed)-10:])
	}
	// Everything between the last ";" and the closing paren must be blank:
	// "( ... ; )" is a syntax error, "( ... )" is an expression.
	beforeParen := strings.TrimSpace(trimmed[:len(trimmed)-1])
	if strings.HasSuffix(beforeParen, ";") {
		t.Errorf("install has a semicolon before the closing parenthesis: %q", beforeParen[len(beforeParen)-20:])
	}
}
