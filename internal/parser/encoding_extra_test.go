package parser

import "testing"

func TestExtractProjectName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "github org/repo",
			input:    "Users-dev-src-github-com-acme-widget",
			expected: "acme-widget",
		},
		{
			name:     "gitlab org/repo",
			input:    "Users-dev-code-gitlab-com-team-service",
			expected: "team-service",
		},
		{
			name:     "bitbucket org/repo",
			input:    "Users-dev-wrk-src-bitbucket-org-company-platform",
			expected: "company-platform",
		},
		{
			name:     "deep nested path truncates to 4",
			input:    "Users-dev-wrk-src-github-com-org-mono-packages-core-lib",
			expected: "mono-packages-core-lib",
		},
		{
			name:     "very deep path limits to last 4",
			input:    "Users-dev-wrk-src-github-com-org-mono-packages-core-lib-util-internal",
			expected: "core-lib-util-internal",
		},
		{
			name:     "home directory pattern",
			input:    "home-ubuntu-projects-webapp",
			expected: "webapp",
		},
		{
			name:     "skip work directories",
			input:    "Users-dev-wrk-src-code-myapp",
			expected: "myapp",
		},
		{
			name:     "skip repos directory",
			input:    "Users-dev-repos-backend",
			expected: "backend",
		},
		{
			name:     "minimal path",
			input:    "app",
			expected: "app",
		},
		{
			name:     "just username",
			input:    "Users-alice",
			expected: "alice",
		},
		{
			name:     "empty parts filtered",
			input:    "Users--dev---project",
			expected: "project",
		},
		{
			name:     "mnt prefix skipped",
			input:    "mnt-data-code-project",
			expected: "project",
		},
		{
			name:     "all skippable parts",
			input:    "Users-dev-wrk-src",
			expected: "src",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractProjectName(tt.input)
			if result != tt.expected {
				t.Errorf("extractProjectName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEncodePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/Users/dev/project", "Users-dev-project"},
		{"/home/user/app", "home-user-app"},
		{"", ""},
		{"relative/path", "relative-path"},
	}

	for _, tt := range tests {
		result := EncodePath(tt.input)
		if result != tt.expected {
			t.Errorf("EncodePath(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	paths := []string{
		"/Users/dev/project",
		"/home/user/src/app",
		"/tmp/work/test",
	}
	for _, path := range paths {
		encoded := EncodePath(path)
		decoded := DecodePath(encoded)
		if decoded != path {
			t.Errorf("roundtrip failed: %q -> %q -> %q", path, encoded, decoded)
		}
	}
}

func TestDecodePathNoLeadingDash(t *testing.T) {
	result := DecodePath("tmp-work-project")
	if result != "/tmp/work/project" {
		t.Errorf("DecodePath without leading dash = %q, want /tmp/work/project", result)
	}
}

func TestGetProjectFullPath(t *testing.T) {
	result := GetProjectFullPath("Users-dev-project")
	if result != "/Users/dev/project" {
		t.Errorf("GetProjectFullPath() = %q, want /Users/dev/project", result)
	}
}
