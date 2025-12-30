package parser

import (
	"os"
	"path/filepath"
	"strings"
)

func DecodePath(encoded string) string {
	if encoded == "" {
		return ""
	}
	decoded := strings.ReplaceAll(encoded, "-", "/")
	if strings.HasPrefix(decoded, "/") {
		return decoded
	}
	return "/" + decoded
}

func EncodePath(path string) string {
	if path == "" {
		return ""
	}
	encoded := strings.ReplaceAll(path, "/", "-")
	if strings.HasPrefix(encoded, "-") {
		return encoded[1:]
	}
	return encoded
}

// GetProjectDisplayName returns a human-readable project name
// Strategy: Check if the actual path exists on filesystem to get real name
// Fallback: Use the encoded directory name directly (more honest than guessing)
func GetProjectDisplayName(encoded string) string {
	if encoded == "" {
		return ""
	}

	// Try to find the actual directory on filesystem
	home, _ := os.UserHomeDir()
	projectsDir := filepath.Join(home, ".claude", "projects", encoded)

	// Check if this encoded path exists as a directory
	if info, err := os.Stat(projectsDir); err == nil && info.IsDir() {
		// The encoded string IS the project identifier
		// Extract meaningful name from it
		return extractProjectName(encoded)
	}

	// Fallback: just extract from encoded string
	return extractProjectName(encoded)
}

// extractProjectName gets a clean display name from the encoded path
// Example: -Users-eric-wrk-src-github-com-org-repo → org-repo
func extractProjectName(encoded string) string {
	parts := strings.Split(encoded, "-")

	// Filter out empty parts first
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if len(nonEmpty) == 0 {
		return encoded
	}

	// Find starting point after common prefixes
	start := 0

	// Skip Users/eric or home/user pattern
	if len(nonEmpty) > start+1 {
		first := strings.ToLower(nonEmpty[start])
		if first == "users" || first == "home" || first == "mnt" {
			start += 2 // skip Users and username
		}
	}

	// Skip wrk, src, work, dev
	for start < len(nonEmpty) {
		p := strings.ToLower(nonEmpty[start])
		if p == "wrk" || p == "src" || p == "work" || p == "dev" || p == "code" || p == "projects" || p == "repos" {
			start++
		} else {
			break
		}
	}

	// Skip github/com pattern
	if start+1 < len(nonEmpty) {
		p := strings.ToLower(nonEmpty[start])
		next := strings.ToLower(nonEmpty[start+1])
		if (p == "github" || p == "gitlab" || p == "bitbucket") &&
			(next == "com" || next == "org" || next == "io") {
			start += 2
		}
	}

	// Get remaining parts
	if start >= len(nonEmpty) {
		return nonEmpty[len(nonEmpty)-1]
	}

	result := nonEmpty[start:]

	// Limit to 4 parts max
	if len(result) > 4 {
		result = result[len(result)-4:]
	}

	return strings.Join(result, "-")
}

// GetProjectFullPath returns the full decoded path for display in tooltips
func GetProjectFullPath(encoded string) string {
	return DecodePath(encoded)
}
