package parser

import "testing"

func TestDecodePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"-Users-eric-projects-myproject", "/Users/eric/projects/myproject"},
		{"-home-user-code-app", "/home/user/code/app"},
		{"", ""},
	}

	for _, tt := range tests {
		result := DecodePath(tt.input)
		if result != tt.expected {
			t.Errorf("DecodePath(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestGetProjectDisplayName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// New format: joined with - for readability
		{"-Users-eric-projects-myproject", "myproject"},
		{"-Users-eric-code-repos-app", "app"},
		{"-home-user-github-com-org-repo", "org-repo"},
		{"-Users-eric-wrk-src-github-com-claude-code-WIP", "claude-code-WIP"},
		{"-Users-eric-wrk-src-github-com-anthropic-claude", "anthropic-claude"},
		{"-Users-eric-wrk-src-github-com-org-repo-subdir-project", "org-repo-subdir-project"},
	}

	for _, tt := range tests {
		result := GetProjectDisplayName(tt.input)
		if result != tt.expected {
			t.Errorf("GetProjectDisplayName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
