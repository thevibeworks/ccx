package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider"
	"github.com/thevibeworks/ccx/internal/render"
)

var exportCmd = &cobra.Command{
	Use:   "export [session]",
	Short: "Export session to file",
	Long: `Export a session to HTML, Markdown, or Org-mode.

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
	exportBrief           bool
	exportShape           string
	exportEnvelope        string
	exportTemplate        string
	exportAll             bool
)

func init() {
	exportCmd.Flags().StringVarP(&exportFormat, "format", "f", "", "output format: html, md, org, exec (default from config)")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "output file path (default: session.<ext>)")
	exportCmd.Flags().StringVarP(&exportProject, "project", "p", "", "project name")
	exportCmd.Flags().BoolVar(&exportAll, "all", false, "search across all projects")
	exportCmd.Flags().StringVar(&exportTheme, "theme", "", "theme: dark, light (default from config)")
	exportCmd.Flags().BoolVar(&exportIncludeThinking, "include-thinking", false, "include thinking blocks")
	exportCmd.Flags().BoolVar(&exportIncludeAgents, "include-agents", false, "include agent sidechains")
	exportCmd.Flags().BoolVarP(&exportBrief, "brief", "b", false, "(deprecated: use --shape=brief) conversation only")
	exportCmd.Flags().StringVar(&exportShape, "shape", "", "content shape: full, brief, trace, exchange (default: full)")
	exportCmd.Flags().StringVar(&exportEnvelope, "envelope", "", "HTML wrapper: standalone, fragment (default: standalone)")
	exportCmd.Flags().StringVar(&exportTemplate, "template", "", "custom template path")
}

func runExport(cmd *cobra.Command, args []string) error {
	backend := provider.Default()

	var session *parser.Session
	var err error

	if len(args) == 0 {
		session, err = selectSession(backend, exportAll)
	} else {
		sessionArg := args[0]
		projectName, sessionID := parseSessionArg(sessionArg)
		if exportProject != "" {
			projectName = exportProject
		}
		query, qErr := sessionLookupQuery(projectName, exportAll)
		if qErr != nil {
			return qErr
		}
		session, err = resolveSessionInQuery(backend, query, sessionID)
	}

	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session not found")
	}

	fullSession, err := backend.ParseSession(session.FilePath)
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

	shape := render.Shape(strings.ToLower(strings.TrimSpace(exportShape)))
	switch shape {
	case "", render.ShapeFull, render.ShapeBrief, render.ShapeTrace, render.ShapeExchange:
	default:
		return fmt.Errorf("invalid shape %q (want full, brief, trace, or exchange)", exportShape)
	}

	envelope := render.Envelope(strings.ToLower(strings.TrimSpace(exportEnvelope)))
	switch envelope {
	case "", render.EnvelopeStandalone, render.EnvelopeFragment:
	default:
		return fmt.Errorf("invalid envelope %q (want standalone or fragment)", exportEnvelope)
	}

	opts := render.ExportOptions{
		Format:          format,
		Theme:           theme,
		IncludeThinking: exportIncludeThinking,
		IncludeAgents:   exportIncludeAgents,
		Brief:           exportBrief,
		Shape:           shape,
		Envelope:        envelope,
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
	case "exec", "exec-md":
		return "-exec.md"
	default:
		return ".html"
	}
}
