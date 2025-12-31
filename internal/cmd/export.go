package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/render"
)

var exportCmd = &cobra.Command{
	Use:   "export [session]",
	Short: "Export session to file",
	Long: `Export a Claude Code session to HTML, Markdown, or Org-mode.

Examples:
  ccx export e38536 --format=html
  ccx export myproject:e38536 -f md -o session.md
  ccx export @1 --format=org`,
	Args: cobra.MaximumNArgs(1),
	RunE: runExport,
}

var (
	exportFormat          string
	exportOutput          string
	exportProject         string
	exportTheme           string
	exportIncludeThinking bool
	exportIncludeAgents   bool
	exportTemplate        string
)

func init() {
	exportCmd.Flags().StringVarP(&exportFormat, "format", "f", "", "output format: html, md, org (default from config)")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "output file path (default: session.<ext>)")
	exportCmd.Flags().StringVarP(&exportProject, "project", "p", "", "project name")
	exportCmd.Flags().StringVar(&exportTheme, "theme", "", "theme: dark, light (default from config)")
	exportCmd.Flags().BoolVar(&exportIncludeThinking, "include-thinking", false, "include thinking blocks")
	exportCmd.Flags().BoolVar(&exportIncludeAgents, "include-agents", false, "include agent sidechains")
	exportCmd.Flags().StringVar(&exportTemplate, "template", "", "custom template path")
}

func runExport(cmd *cobra.Command, args []string) error {
	projectsDir := config.ProjectsDir()

	var session *parser.Session
	var err error

	if len(args) == 0 {
		session, err = selectSession(projectsDir)
	} else {
		sessionArg := args[0]
		projectName, sessionID := parseSessionArg(sessionArg)
		if exportProject != "" {
			projectName = exportProject
		}
		session, err = parser.FindSession(projectsDir, projectName, sessionID)
	}

	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session not found")
	}

	fullSession, err := parser.ParseSession(session.FilePath)
	if err != nil {
		return fmt.Errorf("failed to parse session: %w", err)
	}

	format := exportFormat
	if format == "" {
		format = config.DefaultExportFormat()
	}

	theme := exportTheme
	if theme == "" {
		theme = config.Theme()
	}

	output := exportOutput
	if output == "" {
		ext := formatToExt(format)
		id := session.ID
		if len(id) > 8 {
			id = id[:8]
		}
		output = fmt.Sprintf("session-%s%s", id, ext)
	}

	opts := render.ExportOptions{
		Format:          format,
		Theme:           theme,
		IncludeThinking: exportIncludeThinking,
		IncludeAgents:   exportIncludeAgents,
		TemplatePath:    exportTemplate,
	}

	content, err := render.Export(fullSession, opts)
	if err != nil {
		return fmt.Errorf("failed to render: %w", err)
	}

	if output == "-" {
		fmt.Print(content)
		return nil
	}

	dir := filepath.Dir(output)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	if err := os.WriteFile(output, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Exported to: %s\n", output)
	return nil
}

func formatToExt(format string) string {
	switch strings.ToLower(format) {
	case "html":
		return ".html"
	case "md", "markdown":
		return ".md"
	case "org":
		return ".org"
	default:
		return ".html"
	}
}
