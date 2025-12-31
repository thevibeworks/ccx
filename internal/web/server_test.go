package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thevibeworks/ccx/internal/db"
)

func setupTestDir(t *testing.T) string {
	dir := t.TempDir()

	// Create a test project directory
	projectDir := filepath.Join(dir, "projects", "-test-project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test session file
	sessionFile := filepath.Join(projectDir, "test-session-123.jsonl")
	content := `{"type":"user","timestamp":"2024-01-01T10:00:00Z","uuid":"u1","message":{"content":"Hello"}}
{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","uuid":"a1","parentUuid":"u1","message":{"content":"Hi there!"}}
`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestHandleIndex(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleIndex returned %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("handleIndex returned empty body")
	}
}

func TestHandleIndex_NotFoundForOtherPaths(t *testing.T) {
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()

	handleIndex(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("handleIndex for /nonexistent returned %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleAPIProjects(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	req := httptest.NewRequest("GET", "/api/projects", nil)
	w := httptest.NewRecorder()

	handleAPIProjects(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleAPIProjects returned %d, want %d", w.Code, http.StatusOK)
	}

	var projects []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &projects); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(projects))
	}
}

func TestHandleAPISessions(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	req := httptest.NewRequest("GET", "/api/sessions/-test-project", nil)
	w := httptest.NewRecorder()

	handleAPISessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleAPISessions returned %d, want %d", w.Code, http.StatusOK)
	}

	var sessions []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
}

func TestHandleAPISessions_NotFound(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	req := httptest.NewRequest("GET", "/api/sessions/-nonexistent-project", nil)
	w := httptest.NewRecorder()

	handleAPISessions(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("handleAPISessions for nonexistent project returned %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleAPIStats(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()

	handleAPIStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleAPIStats returned %d, want %d", w.Code, http.StatusOK)
	}

	var stats map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if _, ok := stats["projects"]; !ok {
		t.Error("stats missing projects")
	}
	if _, ok := stats["sessions"]; !ok {
		t.Error("stats missing sessions")
	}
}

func TestHandleAPISearch(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	req := httptest.NewRequest("GET", "/api/search?q=Hello", nil)
	w := httptest.NewRecorder()

	handleAPISearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleAPISearch returned %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if _, ok := result["results"]; !ok {
		t.Error("search result missing 'results' field")
	}
}

func TestHandleAPISearch_EmptyQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/search?q=", nil)
	w := httptest.NewRecorder()

	handleAPISearch(w, req)

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	results, ok := result["results"].([]any)
	if !ok {
		t.Fatal("results is not an array")
	}
	if len(results) != 0 {
		t.Errorf("expected empty results for empty query, got %d", len(results))
	}
}

func TestHandleProject(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	req := httptest.NewRequest("GET", "/project/-test-project", nil)
	w := httptest.NewRecorder()

	handleProject(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleProject returned %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("handleProject returned empty body")
	}
}

func TestHandleProject_NotFound(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	req := httptest.NewRequest("GET", "/project/-nonexistent", nil)
	w := httptest.NewRecorder()

	handleProject(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("handleProject for nonexistent returned %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleSession(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	req := httptest.NewRequest("GET", "/session/-test-project/test-session-123", nil)
	w := httptest.NewRecorder()

	handleSession(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleSession returned %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("handleSession returned empty body")
	}
}

func TestHandleSession_NotFound(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	req := httptest.NewRequest("GET", "/session/-test-project/nonexistent-session", nil)
	w := httptest.NewRecorder()

	handleSession(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("handleSession for nonexistent returned %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleAPIExport_JSON(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	req := httptest.NewRequest("GET", "/api/export/-test-project/test-session-123?format=json", nil)
	w := httptest.NewRecorder()

	handleAPIExport(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleAPIExport returned %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
}

func TestHandleAPIExport_Markdown(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	req := httptest.NewRequest("GET", "/api/export/-test-project/test-session-123?format=md", nil)
	w := httptest.NewRecorder()

	handleAPIExport(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleAPIExport returned %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/markdown") {
		t.Errorf("Content-Type = %q, want text/markdown*", contentType)
	}
}

func TestHandleAPIExport_NotFound(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	req := httptest.NewRequest("GET", "/api/export/-test-project/nonexistent?format=json", nil)
	w := httptest.NewRecorder()

	handleAPIExport(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("handleAPIExport for nonexistent returned %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleStar(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	// Initialize db for stars
	dbPath := filepath.Join(dir, "ccx.db")
	if err := db.Init(dbPath); err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	body := strings.NewReader(`{"action":"add","type":"session","target_id":"test-session-123","project_id":"-test-project"}`)
	req := httptest.NewRequest("POST", "/api/star", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleStar(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleStar returned %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleGetStars(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	// Initialize db for stars
	dbPath := filepath.Join(dir, "ccx.db")
	if err := db.Init(dbPath); err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	req := httptest.NewRequest("GET", "/api/stars", nil)
	w := httptest.NewRecorder()

	handleGetStars(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleGetStars returned %d, want %d", w.Code, http.StatusOK)
	}

	var stars []any
	if err := json.Unmarshal(w.Body.Bytes(), &stars); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
}

func TestHandleAPIFile_AllowsAgentsFile(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	agentFile := filepath.Join(agentsDir, "agent.md")
	if err := os.WriteFile(agentFile, []byte("agent content"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/file?path="+url.QueryEscape(agentFile), nil)
	w := httptest.NewRecorder()

	handleAPIFile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleAPIFile returned %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["content"] != "agent content" {
		t.Fatalf("content = %q, want %q", resp["content"], "agent content")
	}
}

func TestHandleAPIFile_DeniesProjectsFile(t *testing.T) {
	dir := setupTestDir(t)
	projectsDir = filepath.Join(dir, "projects")
	claudeHome = dir

	sessionFile := filepath.Join(projectsDir, "-test-project", "test-session-123.jsonl")
	req := httptest.NewRequest("GET", "/api/file?path="+url.QueryEscape(sessionFile), nil)
	w := httptest.NewRecorder()

	handleAPIFile(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("handleAPIFile returned %d, want %d. Body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestHandleAPIFile_DeniesPrefixConfusion(t *testing.T) {
	dir := t.TempDir()
	claudeHome = dir

	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	evilDir := dir + "_evil"
	if err := os.MkdirAll(evilDir, 0755); err != nil {
		t.Fatal(err)
	}
	evilFile := filepath.Join(evilDir, "evil.md")
	if err := os.WriteFile(evilFile, []byte("nope"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/file?path="+url.QueryEscape(evilFile), nil)
	w := httptest.NewRecorder()

	handleAPIFile(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("handleAPIFile returned %d, want %d. Body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestHandleAPIFile_DeniesSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	claudeHome = dir

	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	outsideDir := filepath.Join(dir, "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(agentsDir, "link.txt")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/file?path="+url.QueryEscape(linkPath), nil)
	w := httptest.NewRecorder()

	handleAPIFile(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("handleAPIFile returned %d, want %d. Body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}
