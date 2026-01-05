package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/db"
	"github.com/thevibeworks/ccx/internal/web"
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start local web server for session browsing",
	Long: `Start ccx web UI - the best way to browse Claude Code sessions.

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
	webPort   int
	webHost   string
	webNoOpen bool
)

func init() {
	webCmd.Flags().IntVarP(&webPort, "port", "p", 8080, "port to listen on")
	webCmd.Flags().StringVar(&webHost, "host", "localhost", "host to bind to")
	webCmd.Flags().BoolVar(&webNoOpen, "no-open", false, "don't open browser automatically")

	rootCmd.AddCommand(webCmd)
}

func runWeb(cmd *cobra.Command, args []string) error {
	projectsDir := config.ProjectsDir()
	addr := fmt.Sprintf("%s:%d", webHost, webPort)
	url := fmt.Sprintf("http://%s", addr)

	// Initialize database
	dataDir := config.DataDir()
	if err := db.Init(dataDir); err != nil {
		fmt.Printf("Warning: Could not initialize database: %v\n", err)
	}
	defer db.Close()

	fmt.Printf("Starting ccx web server...\n")
	fmt.Printf("Projects: %s\n", projectsDir)
	fmt.Printf("Database: %s\n", dataDir)
	fmt.Printf("URL: %s\n\n", url)

	if !webNoOpen {
		go func() {
			openBrowser(url)
		}()
	}

	return web.Serve(addr, projectsDir)
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
