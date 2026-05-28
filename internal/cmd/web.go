package cmd

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/db"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider"
	"github.com/thevibeworks/ccx/internal/web"
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start local web server for session browsing",
	Long: `Start ccx web UI for browsing agent sessions.

Deep-link flags:
  --project [path]   Open directly to a project page
  --session <id>     Open directly to a session (full UUID or short prefix)
  --latest           Open to the most recent session in the current workspace

If the server is already running on the target port, these flags open
the browser without starting a second server.

Features:
  - Project/session browser with global search
  - Tree-aware session viewer with message threading
  - Collapsible thinking blocks and tool calls
  - In-session search with filter chips (User/Response/Tools/Agents)
  - Live tail mode for active sessions (auto-refresh)
  - Syntax highlighting for code blocks
  - Dark/light theme toggle (press 'd')
  - Keyboard navigation (j/k scroll, / search, z fold, r refresh)

Opens browser automatically. Use --no-open to disable.`,
	RunE: runWeb,
}

var (
	webPort    int
	webHost    string
	webNoOpen  bool
	webProject string
	webSession string
	webLatest  bool
)

func init() {
	webCmd.Flags().IntVarP(&webPort, "port", "p", 8080, "port to listen on")
	webCmd.Flags().StringVar(&webHost, "host", "localhost", "host to bind to")
	webCmd.Flags().BoolVar(&webNoOpen, "no-open", false, "don't open browser automatically")
	webCmd.Flags().StringVar(&webProject, "project", "", "open project page (path or name; empty = current workspace)")
	webCmd.Flags().Lookup("project").NoOptDefVal = "."
	webCmd.Flags().StringVar(&webSession, "session", "", "open a specific session by ID or short prefix")
	webCmd.Flags().BoolVar(&webLatest, "latest", false, "open the most recent session in the current workspace")
	webCmd.MarkFlagsMutuallyExclusive("project", "session", "latest")

	rootCmd.AddCommand(webCmd)
}

func runWeb(cmd *cobra.Command, args []string) error {
	backend := provider.Default()
	addr := fmt.Sprintf("%s:%d", webHost, webPort)
	baseURL := fmt.Sprintf("http://%s", addr)

	deepPath, err := resolveDeepLink(backend, cmd)
	if err != nil {
		return err
	}
	targetURL := baseURL + deepPath

	if serverAlive(baseURL) {
		if deepPath != "" {
			fmt.Printf("Server already running at %s\n", baseURL)
			fmt.Printf("Opening: %s\n", targetURL)
		} else {
			fmt.Printf("Server already running, opening: %s\n", baseURL)
		}
		if !webNoOpen {
			openBrowser(targetURL)
		}
		return nil
	}

	dataDir := config.DataDir()
	if err := db.Init(dataDir); err != nil {
		fmt.Printf("Warning: Could not initialize database: %v\n", err)
	}
	defer db.Close()

	fmt.Printf("Starting ccx web server...\n")
	for _, home := range backend.Homes() {
		fmt.Printf("Source: %s\n", home)
	}
	fmt.Printf("Database: %s\n", dataDir)
	if deepPath != "" {
		fmt.Printf("URL: %s\n\n", targetURL)
	} else {
		fmt.Printf("URL: %s\n\n", baseURL)
	}

	if !webNoOpen {
		openBrowser(targetURL)
	}

	return web.Serve(addr, backend)
}

func resolveDeepLink(backend provider.Backend, cmd *cobra.Command) (string, error) {
	projectFlagSet := cmd.Flags().Changed("project")

	switch {
	case webSession != "":
		return resolveSessionLink(backend, webSession)
	case webLatest:
		return resolveLatestLink(backend)
	case projectFlagSet:
		return resolveProjectLink(backend, webProject)
	default:
		return "", nil
	}
}

func resolveProjectLink(backend provider.Backend, path string) (string, error) {
	if path == "" || path == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("--project: %w", err)
		}
		path = cwd
	}

	project, err := findProjectByPath(backend, path)
	if err != nil {
		return "", fmt.Errorf("--project: %w", err)
	}
	if project == nil {
		return "", fmt.Errorf("--project: no project found for %s", path)
	}

	return "/project/" + project.EncodedName, nil
}

func resolveSessionLink(backend provider.Backend, sessionID string) (string, error) {
	projects, err := backend.DiscoverProjects()
	if err != nil {
		return "", fmt.Errorf("--session: %w", err)
	}

	// Collect all sessions, try exact then prefix match
	var allSessions []sessionWithProject
	for _, p := range projects {
		for _, s := range p.Sessions {
			allSessions = append(allSessions, sessionWithProject{session: s, project: p})
		}
	}

	// Exact match first
	for _, sp := range allSessions {
		if sp.session.ID == sessionID {
			return "/session/" + sp.project.EncodedName + "/" + sp.session.ID, nil
		}
	}

	// Prefix match
	var matches []sessionWithProject
	for _, sp := range allSessions {
		if strings.HasPrefix(sp.session.ID, sessionID) {
			matches = append(matches, sp)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("--session: no session matches %q", sessionID)
	case 1:
		sp := matches[0]
		return "/session/" + sp.project.EncodedName + "/" + sp.session.ID, nil
	default:
		var ids []string
		for _, sp := range matches {
			short := sp.session.ID
			if len(short) > 12 {
				short = short[:12]
			}
			ids = append(ids, short)
		}
		return "", fmt.Errorf("--session: %q is ambiguous, matches %d sessions: %s",
			sessionID, len(matches), strings.Join(ids, ", "))
	}
}

type sessionWithProject struct {
	session *parser.Session
	project *parser.Project
}

func resolveLatestLink(backend provider.Backend) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("--latest: %w", err)
	}

	project, err := findProjectByPath(backend, cwd)
	if err != nil {
		return "", fmt.Errorf("--latest: %w", err)
	}
	if project == nil {
		return "", fmt.Errorf("--latest: no project found for current workspace")
	}

	if len(project.Sessions) == 0 {
		return "", fmt.Errorf("--latest: project has no sessions")
	}

	latest := project.Sessions[0]
	for _, s := range project.Sessions[1:] {
		if s.EndTime.After(latest.EndTime) {
			latest = s
		}
	}

	return "/session/" + project.EncodedName + "/" + latest.ID, nil
}

func findProjectByPath(backend provider.Backend, path string) (*parser.Project, error) {
	projects, err := backend.DiscoverProjects()
	if err != nil {
		return nil, err
	}

	for _, p := range projects {
		if catalog.ProjectMatchesWorkspace(p, path) {
			return p, nil
		}
	}

	for _, p := range projects {
		if catalog.ProjectMatchesName(p, path) {
			return p, nil
		}
	}

	return nil, nil
}

func serverAlive(baseURL string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/api/stats")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
