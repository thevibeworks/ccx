package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/claude-code/ccx/internal/config"
	"github.com/claude-code/ccx/internal/db"
	"github.com/claude-code/ccx/internal/web"
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start local web server",
	Long: `Start a local web server for interactive session browsing.

Features:
  - Project/session browser
  - Tree-aware session viewer
  - Collapsible thinking/tool blocks
  - Syntax highlighting
  - Dark/light toggle
  - Keyboard navigation (j/k, /, gg/G)
  - Search across sessions`,
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
