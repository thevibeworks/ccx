package render

import (
	"strings"
	"testing"
)

// Embedded escapes in session content must never reach the terminal:
// they retitle windows and flip grep into binary mode even under
// --color=never.
func TestSanitizeContent(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"sgr", "\x1b[1mbold\x1b[0m plain", "bold plain"},
		{"osc title", "\x1b]0;evil\x07after", "after"},
		{"stray esc and nul", "a\x1bb\x00c", "abc"},
		{"keeps newline and tab", "line1\n\tline2", "line1\n\tline2"},
		{"clean passthrough", "just text", "just text"},
	}
	for _, tc := range cases {
		if got := sanitizeContent(tc.in); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSanitizeContentNoControlBytesSurvive(t *testing.T) {
	in := "x\x01\x02\x7f\x1b[31my\x1b]2;t\x1b\\z"
	got := sanitizeContent(in)
	for _, r := range got {
		if r < 0x20 && r != '\n' && r != '\t' {
			t.Fatalf("control byte %q survived in %q", r, got)
		}
	}
	if !strings.Contains(got, "x") || !strings.Contains(got, "y") || !strings.Contains(got, "z") {
		t.Fatalf("printable content lost: %q", got)
	}
}
