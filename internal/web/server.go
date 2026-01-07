package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/db"
	"github.com/thevibeworks/ccx/internal/parser"
)

var (
	projectsDir string
	claudeHome  string
)

type Settings struct {
	Env              map[string]string      `json:"env"`
	Permissions      map[string]string      `json:"permissions"`
	Hooks            map[string]interface{} `json:"hooks"`
	StatusLine       map[string]interface{} `json:"statusLine"`
	EnabledPlugins   map[string]bool        `json:"enabledPlugins"`
	PromptSuggestion bool                   `json:"promptSuggestionEnabled"`
}

type AgentInfo struct {
	Name     string
	FilePath string
}

type SkillInfo struct {
	Name string
	Path string
}

type GlobalConfig struct {
	NumStartups int    `json:"numStartups"`
	Theme       string `json:"theme"`
	Verbose     bool   `json:"verbose"`
	EditorMode  string `json:"editorMode"`
}

func Serve(addr, projDir string) error {
	projectsDir = projDir
	claudeHome = config.ClaudeHome()

	mux := http.NewServeMux()

	// Pages
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/project/", handleProject)
	mux.HandleFunc("/session/", handleSession)
	mux.HandleFunc("/settings", handleSettings)
	mux.HandleFunc("/search", handleSearchPage)

	// API
	mux.HandleFunc("/api/projects", handleAPIProjects)
	mux.HandleFunc("/api/sessions/", handleAPISessions)
	mux.HandleFunc("/api/session/", handleAPISession)
	mux.HandleFunc("/api/stats", handleAPIStats)
	mux.HandleFunc("/api/settings", handleAPISettings)
	mux.HandleFunc("/api/export/", handleAPIExport)
	mux.HandleFunc("/api/search", handleAPISearch)

	// SSE for realtime updates
	mux.HandleFunc("/api/watch/", handleWatch)

	// Star/favorite endpoints
	mux.HandleFunc("/api/star", handleStar)
	mux.HandleFunc("/api/stars", handleGetStars)

	// File content API (for agents/skills)
	mux.HandleFunc("/api/file", handleAPIFile)

	// Wrap with logging middleware
	handler := logRequest(mux)

	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // disabled for SSE and large exports
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("ccx web server listening on http://%s", addr)
	return server.ListenAndServe()
}

// logRequest logs HTTP requests with method, path, and duration
func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		// Skip logging for SSE (long-running)
		if !strings.HasPrefix(r.URL.Path, "/api/watch/") {
			log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	projects, err := parser.DiscoverProjects(projectsDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get query params for filtering/sorting
	q := r.URL.Query()
	search := strings.ToLower(q.Get("q"))
	sortBy := q.Get("sort")
	if sortBy == "" {
		sortBy = "time"
	}

	// Filter
	if search != "" {
		var filtered []*parser.Project
		for _, p := range projects {
			if strings.Contains(strings.ToLower(p.Name), search) {
				filtered = append(filtered, p)
			}
		}
		projects = filtered
	}

	// Sort
	switch sortBy {
	case "name":
		sort.Slice(projects, func(i, j int) bool {
			return projects[i].Name < projects[j].Name
		})
	case "sessions":
		sort.Slice(projects, func(i, j int) bool {
			return len(projects[i].Sessions) > len(projects[j].Sessions)
		})
	default: // time
		sort.Slice(projects, func(i, j int) bool {
			return projects[i].LastModified.After(projects[j].LastModified)
		})
	}

	// Calculate stats
	totalSessions := 0
	for _, p := range projects {
		totalSessions += len(p.Sessions)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderIndexPage(projects, totalSessions, search, sortBy))
}

func handleProject(w http.ResponseWriter, r *http.Request) {
	encodedName := strings.TrimPrefix(r.URL.Path, "/project/")
	if encodedName == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	project, err := parser.FindProject(projectsDir, encodedName)
	if err != nil || project == nil {
		http.NotFound(w, r)
		return
	}

	// Fetch all projects for left nav
	allProjects, _ := parser.DiscoverProjects(projectsDir)
	sort.Slice(allProjects, func(i, j int) bool {
		return allProjects[i].LastModified.After(allProjects[j].LastModified)
	})

	// Get query params
	q := r.URL.Query()
	search := strings.ToLower(q.Get("q"))
	sortBy := q.Get("sort")
	if sortBy == "" {
		sortBy = "time"
	}

	sessions := project.Sessions

	// Filter
	if search != "" {
		var filtered []*parser.Session
		for _, s := range sessions {
			if strings.Contains(strings.ToLower(s.Summary), search) ||
				strings.Contains(strings.ToLower(s.ID), search) {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
	}

	// Sort
	switch sortBy {
	case "messages":
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].Stats.MessageCount > sessions[j].Stats.MessageCount
		})
	default: // time
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].EndTime.After(sessions[j].EndTime)
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderProjectPage(project, sessions, allProjects, search, sortBy))
}

func handleSession(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/session/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	projectName, sessionID := parts[0], parts[1]
	session, err := parser.FindSession(projectsDir, projectName, sessionID)
	if err != nil || session == nil {
		http.NotFound(w, r)
		return
	}

	fullSession, err := parser.ParseSession(session.FilePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch project's sessions for left nav
	project, _ := parser.FindProject(projectsDir, projectName)
	var allSessions []*parser.Session
	if project != nil {
		allSessions = project.Sessions
		sort.Slice(allSessions, func(i, j int) bool {
			return allSessions[i].EndTime.After(allSessions[j].EndTime)
		})
	}

	// Get display options from query
	q := r.URL.Query()
	showThinking := q.Get("thinking") == "1"
	showTools := q.Get("tools") == "1" // default: only active tools expanded
	loadAll := q.Get("all") == "1"     // Load all messages (no progressive loading)
	theme := q.Get("theme")
	if theme == "" {
		theme = config.Theme() // respect user config
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderSessionPage(fullSession, projectName, allSessions, showThinking, showTools, loadAll, theme))
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	settings := loadSettings()
	globalConfig := loadGlobalConfig()
	agents := loadAgents()
	skills := loadSkills()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderSettingsPage(settings, globalConfig, agents, skills))
}

func loadAgents() []AgentInfo {
	agentsDir := filepath.Join(claudeHome, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return nil
	}

	var agents []AgentInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		agents = append(agents, AgentInfo{
			Name:     name,
			FilePath: filepath.Join(agentsDir, entry.Name()),
		})
	}
	return agents
}

func loadSkills() []SkillInfo {
	skillsDir := filepath.Join(claudeHome, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}

	var skills []SkillInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skills = append(skills, SkillInfo{
			Name: entry.Name(),
			Path: filepath.Join(skillsDir, entry.Name()),
		})
	}
	return skills
}

func handleWatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/watch/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	projectName, sessionID := parts[0], parts[1]
	session, err := parser.FindSession(projectsDir, projectName, sessionID)
	if err != nil || session == nil {
		http.NotFound(w, r)
		return
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Get initial file size
	filePath := session.FilePath
	stat, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lastSize := stat.Size()

	// Send initial connection message
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"watching\"}\n\n")
	flusher.Flush()

	const maxChunkSize = 1 << 20 // 1MB cap to prevent DoS
	var partialLine string

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Check if file has grown
			stat, err := os.Stat(filePath)
			if err != nil {
				continue
			}
			newSize := stat.Size()
			if newSize <= lastSize {
				continue
			}

			// Cap chunk size to prevent huge allocations
			chunkSize := newSize - lastSize
			if chunkSize > maxChunkSize {
				chunkSize = maxChunkSize
			}

			// Read new content
			file, err := os.Open(filePath)
			if err != nil {
				continue
			}
			if _, err := file.Seek(lastSize, 0); err != nil {
				file.Close()
				continue
			}
			newBytes := make([]byte, chunkSize)
			n, err := file.Read(newBytes)
			file.Close()
			if err != nil || n == 0 {
				continue
			}
			lastSize += int64(n)

			// Combine with any partial line from previous read
			data := partialLine + string(newBytes[:n])
			partialLine = ""

			// Send each complete line
			lines := strings.Split(data, "\n")
			for i, line := range lines {
				// Last element may be partial if chunk didn't end with newline
				if i == len(lines)-1 && !strings.HasSuffix(data, "\n") {
					partialLine = line
					continue
				}
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				// Send the actual JSONL line
				fmt.Fprintf(w, "event: line\ndata: %s\n\n", line)
				flusher.Flush()
			}
		}
	}
}

func handleAPIProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := parser.DiscoverProjects(projectsDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type projectResp struct {
		Name         string `json:"name"`
		EncodedName  string `json:"encoded_name"`
		Sessions     int    `json:"sessions"`
		LastModified string `json:"last_modified"`
	}

	resp := make([]projectResp, len(projects))
	for i, p := range projects {
		resp[i] = projectResp{
			Name:         p.Name,
			EncodedName:  p.EncodedName,
			Sessions:     len(p.Sessions),
			LastModified: p.LastModified.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func handleAPISessions(w http.ResponseWriter, r *http.Request) {
	encodedName := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	project, err := parser.FindProject(projectsDir, encodedName)
	if err != nil || project == nil {
		http.NotFound(w, r)
		return
	}

	type sessionResp struct {
		ID        string `json:"id"`
		Summary   string `json:"summary"`
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
	}

	resp := make([]sessionResp, len(project.Sessions))
	for i, s := range project.Sessions {
		resp[i] = sessionResp{
			ID:        s.ID,
			Summary:   s.Summary,
			StartTime: s.StartTime.Format(time.RFC3339),
			EndTime:   s.EndTime.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func handleAPISession(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/session/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	projectName, sessionID := parts[0], parts[1]
	session, err := parser.FindSession(projectsDir, projectName, sessionID)
	if err != nil || session == nil {
		http.NotFound(w, r)
		return
	}

	fullSession, err := parser.ParseSession(session.FilePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fullSession)
}

func handleAPIStats(w http.ResponseWriter, r *http.Request) {
	projects, err := parser.DiscoverProjects(projectsDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalSessions := 0
	for _, p := range projects {
		totalSessions += len(p.Sessions)
	}

	resp := map[string]interface{}{
		"projects":    len(projects),
		"sessions":    totalSessions,
		"claude_home": claudeHome,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func handleAPISettings(w http.ResponseWriter, r *http.Request) {
	settings := loadSettings()
	globalConfig := loadGlobalConfig()

	resp := map[string]interface{}{
		"settings":      settings,
		"global_config": globalConfig,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func loadSettings() *Settings {
	settingsPath := filepath.Join(claudeHome, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil
	}
	return &settings
}

func loadGlobalConfig() *GlobalConfig {
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".claude.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var config GlobalConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}
	return &config
}

func handleStar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Action    string `json:"action"` // "add" or "remove"
		Type      string `json:"type"`   // "project", "session", "message"
		TargetID  string `json:"target_id"`
		ProjectID string `json:"project_id"`
		Note      string `json:"note"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var err error
	switch req.Action {
	case "add":
		err = db.AddStar(req.Type, req.TargetID, req.ProjectID, req.Note)
	case "remove":
		err = db.RemoveStar(req.Type, req.TargetID)
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func handleGetStars(w http.ResponseWriter, r *http.Request) {
	itemType := r.URL.Query().Get("type")

	var stars []db.Star
	var err error
	if itemType != "" {
		stars, err = db.GetStars(itemType)
	} else {
		stars, err = db.GetAllStars()
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stars)
}

func handleAPIFile(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	absPath = filepath.Clean(absPath)
	info, err := os.Stat(absPath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if info.IsDir() {
		http.Error(w, "path is a directory", http.StatusBadRequest)
		return
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	allowedRoots := []string{
		filepath.Join(claudeHome, "agents"),
		filepath.Join(claudeHome, "skills"),
	}
	allowed := false
	for _, root := range allowedRoots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		resolvedRoot, err := filepath.EvalSymlinks(filepath.Clean(absRoot))
		if err != nil {
			continue
		}
		if isSubpath(resolvedPath, resolvedRoot) {
			allowed = true
			break
		}
	}
	if !allowed {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"path":    resolvedPath,
		"content": string(content),
	})
}

func isSubpath(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." || rel == "" {
		return true
	}
	if rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func handleSearchPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderSearchPage(projectsDir, query))
}

func handleAPIExport(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/export/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	projectName, sessionID := parts[0], parts[1]
	session, err := parser.FindSession(projectsDir, projectName, sessionID)
	if err != nil || session == nil {
		http.NotFound(w, r)
		return
	}

	fullSession, err := parser.ParseSession(session.FilePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=session-%s.json", truncate(sessionID, 8)))
		_ = json.NewEncoder(w).Encode(fullSession)
	case "html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=session-%s.html", truncate(sessionID, 8)))
		fmt.Fprint(w, renderSessionPage(fullSession, projectName, nil, true, true, true, "light"))
	case "md", "markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=session-%s.md", truncate(sessionID, 8)))
		fmt.Fprint(w, exportMarkdown(fullSession))
	case "org":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=session-%s.org", truncate(sessionID, 8)))
		fmt.Fprint(w, exportOrg(fullSession))
	case "txt", "text":
		// CLI-style export matching /export format
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		filename := generateExportFilename(fullSession)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
		fmt.Fprint(w, exportTxt(fullSession))
	default:
		http.Error(w, "Invalid format", http.StatusBadRequest)
	}
}

func handleAPISearch(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("q"))
	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
		return
	}

	projects, err := parser.DiscoverProjects(projectsDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type searchResult struct {
		URL       string `json:"url"`
		Summary   string `json:"summary"`
		Project   string `json:"project"`
		Time      string `json:"time"`
		Type      string `json:"type"`
		Snippet   string `json:"snippet"`
		MessageID string `json:"message_id,omitempty"`
		Priority  int    `json:"priority"`
	}

	var results []searchResult
	maxResults := 30

	for _, p := range projects {
		projDisplay := parser.GetProjectDisplayName(p.EncodedName)
		projPath := parser.DecodePath(p.EncodedName)

		// Priority 0: Exact session/project ID match
		if strings.EqualFold(p.EncodedName, query) || strings.Contains(p.EncodedName, query) {
			results = append(results, searchResult{
				URL:      fmt.Sprintf("/project/%s", p.EncodedName),
				Summary:  projDisplay,
				Project:  projDisplay,
				Type:     "project",
				Priority: 0,
			})
		}

		// Priority 1: Project path contains query
		if strings.Contains(strings.ToLower(projPath), query) && !strings.Contains(p.EncodedName, query) {
			results = append(results, searchResult{
				URL:      fmt.Sprintf("/project/%s", p.EncodedName),
				Summary:  projDisplay,
				Project:  projDisplay,
				Type:     "project",
				Snippet:  projPath,
				Priority: 1,
			})
		}

		for _, s := range p.Sessions {
			// Priority 0: Exact session ID match
			if strings.EqualFold(s.ID, query) || strings.HasPrefix(strings.ToLower(s.ID), query) {
				results = append(results, searchResult{
					URL:      fmt.Sprintf("/session/%s/%s", p.EncodedName, s.ID),
					Summary:  truncateSummary(s.Summary, 80),
					Project:  projDisplay,
					Time:     formatAge(s.StartTime),
					Type:     "session",
					Priority: 0,
				})
				continue
			}

			// Priority 2: Session summary match
			if strings.Contains(strings.ToLower(s.Summary), query) {
				results = append(results, searchResult{
					URL:      fmt.Sprintf("/session/%s/%s", p.EncodedName, s.ID),
					Summary:  truncateSummary(s.Summary, 80),
					Project:  projDisplay,
					Time:     formatAge(s.StartTime),
					Type:     "session",
					Priority: 2,
				})
				continue
			}

			// Priority 3: Deep search - parse session and search content
			if len(results) < maxResults {
				fullSession, err := parser.ParseSession(s.FilePath)
				if err != nil {
					continue
				}
				snippet, msgID := searchSessionContent(fullSession, query)
				if snippet != "" {
					url := fmt.Sprintf("/session/%s/%s", p.EncodedName, s.ID)
					if msgID != "" {
						url += "#msg-" + msgID
					}
					results = append(results, searchResult{
						URL:       url,
						Summary:   truncateSummary(s.Summary, 60),
						Project:   projDisplay,
						Time:      formatAge(s.StartTime),
						Type:      "message",
						Snippet:   snippet,
						MessageID: msgID,
						Priority:  3,
					})
				}
			}
		}
	}

	// Sort by priority then time
	sort.Slice(results, func(i, j int) bool {
		if results[i].Priority != results[j].Priority {
			return results[i].Priority < results[j].Priority
		}
		return i < j
	})

	// Limit results
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}

// No truncation - return full summary
func truncateSummary(s string, n int) string {
	return s
}

// searchSessionContent returns (snippet, messageID) for first match
func searchSessionContent(s *parser.Session, query string) (string, string) {
	allMsgs := flattenMessages(s.RootMessages)
	for _, msg := range allMsgs {
		for _, block := range msg.Content {
			if block.Type == "text" {
				lower := strings.ToLower(block.Text)
				if strings.Contains(lower, query) {
					return extractSnippet(block.Text, query, 60), msg.UUID
				}
			}
			if block.Type == "tool_use" {
				if inputJSON, err := json.Marshal(block.ToolInput); err == nil {
					if strings.Contains(strings.ToLower(string(inputJSON)), query) {
						return fmt.Sprintf("[%s] %s", block.ToolName, extractSnippet(string(inputJSON), query, 40)), msg.UUID
					}
				}
			}
			if block.Type == "tool_result" {
				resultStr := fmt.Sprintf("%v", block.ToolResult)
				if strings.Contains(strings.ToLower(resultStr), query) {
					return extractSnippet(resultStr, query, 60), msg.UUID
				}
			}
		}
	}
	return "", ""
}

func extractSnippet(text, query string, maxLen int) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, query)
	if idx == -1 {
		return ""
	}
	start := idx - 20
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + maxLen - 20
	if end > len(text) {
		end = len(text)
	}
	snippet := text[start:end]
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet = snippet + "..."
	}
	return snippet
}

func flattenMessages(messages []*parser.Message) []*parser.Message {
	var result []*parser.Message
	var flatten func(msgs []*parser.Message)
	flatten = func(msgs []*parser.Message) {
		for _, msg := range msgs {
			result = append(result, msg)
			if len(msg.Children) > 0 {
				flatten(msg.Children)
			}
		}
	}
	flatten(messages)
	return result
}

func exportMarkdown(s *parser.Session) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Session %s\n\n", s.ID))
	b.WriteString(fmt.Sprintf("**Started:** %s\n\n", s.StartTime.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("**Messages:** %d | **Tools:** %d\n\n---\n\n", s.Stats.MessageCount, s.Stats.ToolCalls))

	// Flatten and export with proper hierarchy based on Kind
	allMsgs := flattenMessages(s.RootMessages)
	for _, msg := range allMsgs {
		exportMessageMd(&b, msg)
	}
	return b.String()
}

func exportMessageMd(b *strings.Builder, msg *parser.Message) {
	// Use Kind for proper formatting, not depth-based indentation
	switch msg.Kind {
	case parser.KindCompactSummary:
		b.WriteString("---\n## ◇ CONTEXT COMPACTED\n\n")
	case parser.KindUserPrompt:
		b.WriteString(fmt.Sprintf("## ▶ USER (%s)\n\n", msg.Timestamp.Format("15:04:05")))
	case parser.KindCommand:
		b.WriteString(fmt.Sprintf("## ⌘ %s (%s)\n\n", msg.CommandName, msg.Timestamp.Format("15:04:05")))
		if msg.CommandArgs != "" {
			b.WriteString(msg.CommandArgs + "\n\n")
		}
		return
	case parser.KindMeta:
		b.WriteString(fmt.Sprintf("> *System Instructions* (%s)\n\n", msg.Timestamp.Format("15:04:05")))
	case parser.KindAssistant:
		b.WriteString(fmt.Sprintf("### ● ASSISTANT (%s)\n\n", msg.Timestamp.Format("15:04:05")))
	case parser.KindToolResult:
		// Tool results are inline, no header
	default:
		b.WriteString(fmt.Sprintf("### %s (%s)\n\n", msg.Type, msg.Timestamp.Format("15:04:05")))
	}

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			b.WriteString(block.Text + "\n\n")
		case "thinking":
			b.WriteString("> *∴ Thinking...*\n\n")
		case "tool_use":
			b.WriteString(fmt.Sprintf("#### ● %s\n\n", block.ToolName))
			if block.ToolInput != nil {
				if inputJSON, err := json.MarshalIndent(block.ToolInput, "", "  "); err == nil {
					b.WriteString("```json\n" + string(inputJSON) + "\n```\n\n")
				}
			}
		case "tool_result":
			result := fmt.Sprintf("%v", block.ToolResult)
			b.WriteString("```\n" + result + "\n```\n\n")
		}
	}
}

func exportOrg(s *parser.Session) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("#+TITLE: Session %s\n", s.ID))
	b.WriteString(fmt.Sprintf("#+DATE: %s\n\n", s.StartTime.Format("2006-01-02")))

	// Flatten and export with proper hierarchy based on Kind
	allMsgs := flattenMessages(s.RootMessages)
	for _, msg := range allMsgs {
		exportMessageOrg(&b, msg)
	}
	return b.String()
}

func exportMessageOrg(b *strings.Builder, msg *parser.Message) {
	// Use Kind for proper formatting, max 3 levels
	switch msg.Kind {
	case parser.KindCompactSummary:
		b.WriteString("* ◇ CONTEXT COMPACTED\n")
	case parser.KindUserPrompt:
		b.WriteString(fmt.Sprintf("* ▶ USER [%s]\n", msg.Timestamp.Format("15:04:05")))
	case parser.KindCommand:
		b.WriteString(fmt.Sprintf("* ⌘ %s [%s]\n", msg.CommandName, msg.Timestamp.Format("15:04:05")))
		if msg.CommandArgs != "" {
			b.WriteString(msg.CommandArgs + "\n")
		}
		return
	case parser.KindMeta:
		b.WriteString(fmt.Sprintf("** System Instructions [%s]\n", msg.Timestamp.Format("15:04:05")))
	case parser.KindAssistant:
		b.WriteString(fmt.Sprintf("** ● ASSISTANT [%s]\n", msg.Timestamp.Format("15:04:05")))
	case parser.KindToolResult:
		// Tool results inline
	default:
		b.WriteString(fmt.Sprintf("** %s [%s]\n", msg.Type, msg.Timestamp.Format("15:04:05")))
	}

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			b.WriteString(block.Text + "\n")
		case "thinking":
			b.WriteString("/∴ Thinking.../\n")
		case "tool_use":
			b.WriteString(fmt.Sprintf("*** ● %s\n", block.ToolName))
			if block.ToolInput != nil {
				if inputJSON, err := json.MarshalIndent(block.ToolInput, "", "  "); err == nil {
					b.WriteString("#+BEGIN_SRC json\n" + string(inputJSON) + "\n#+END_SRC\n")
				}
			}
		case "tool_result":
			result := fmt.Sprintf("%v", block.ToolResult)
			b.WriteString("#+BEGIN_EXAMPLE\n" + result + "\n#+END_EXAMPLE\n")
		}
	}
}

// generateExportFilename creates a filename matching /export CLI style
// Format: YYYY-MM-DD-first-words-of-summary.txt
func generateExportFilename(s *parser.Session) string {
	date := s.StartTime.Format("2006-01-02")
	summary := strings.ToLower(s.Summary)
	// Remove special characters, keep only alphanumeric and spaces
	var clean strings.Builder
	for _, r := range summary {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			clean.WriteRune(r)
		}
	}
	words := strings.Fields(clean.String())
	if len(words) > 6 {
		words = words[:6]
	}
	slug := strings.Join(words, "-")
	if len(slug) > 50 {
		slug = slug[:50]
	}
	if slug == "" {
		slug = "session"
	}
	return fmt.Sprintf("%s-%s.txt", date, slug)
}

// exportTxt exports session in CLI-style text format
func exportTxt(s *parser.Session) string {
	var b strings.Builder

	// Header like CLI export
	b.WriteString("\n")
	b.WriteString(" * ▐▛███▜▌ *   Claude Code\n")
	b.WriteString("* ▝▜█████▛▘ *\n")
	b.WriteString(" *  ▘▘ ▝▝  *\n")
	b.WriteString("\n")

	allMsgs := flattenMessages(s.RootMessages)
	for _, msg := range allMsgs {
		exportMessageTxt(&b, msg)
	}
	return b.String()
}

func exportMessageTxt(b *strings.Builder, msg *parser.Message) {
	switch msg.Kind {
	case parser.KindCompactSummary:
		b.WriteString("══════════════════ Conversation compacted ═════════════════\n\n")
	case parser.KindUserPrompt:
		b.WriteString(fmt.Sprintf("> %s\n\n", getFirstTextContent(msg)))
	case parser.KindCommand:
		b.WriteString(fmt.Sprintf("> %s %s\n\n", msg.CommandName, msg.CommandArgs))
	case parser.KindMeta:
		// Skip meta in text export
	case parser.KindAssistant:
		b.WriteString("● " + getFirstTextContent(msg) + "\n\n")
		for _, block := range msg.Content {
			if block.Type == "tool_use" {
				preview := ""
				if m, ok := block.ToolInput.(map[string]any); ok {
					if p, ok := m["pattern"].(string); ok {
						preview = p
					} else if p, ok := m["command"].(string); ok {
						preview = p
					} else if p, ok := m["file_path"].(string); ok {
						preview = p
					}
				}
				b.WriteString(fmt.Sprintf("● %s(%s)\n", block.ToolName, preview))
			}
		}
	case parser.KindToolResult:
		// Skip standalone tool results
	}
}

func getFirstTextContent(msg *parser.Message) string {
	for _, block := range msg.Content {
		if block.Type == "text" && block.Text != "" {
			return block.Text
		}
	}
	return ""
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}
