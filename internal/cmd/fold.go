package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/fold"
	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider"
)

var foldCmd = &cobra.Command{
	Use:   "fold [session]",
	Short: "Fold a session into structured decisions",
	Long: `Analyze a session and produce structured decision data.

Parses the session into turns, detects corrections, tracks file mutations,
and correlates with git commits in the session time window. Outputs JSON
for consumption by the ccx-fold Claude Code skill or other tools.

Examples:
  ccx fold                     # Latest session, JSON to stdout
  ccx fold e38536              # Specific session
  ccx fold -o fold.json        # Write to file
  ccx fold --html              # Also generate HTML review to temp dir`,
	Args: cobra.MaximumNArgs(1),
	RunE: runFold,
}

var (
	foldOutput  string
	foldProject string
	foldHTML    bool
	foldAll     bool
)

func init() {
	foldCmd.Flags().StringVarP(&foldOutput, "output", "o", "", "output file (default: stdout)")
	foldCmd.Flags().StringVarP(&foldProject, "project", "p", "", "project name")
	foldCmd.Flags().BoolVar(&foldAll, "all", false, "search across all projects")
	foldCmd.Flags().BoolVar(&foldHTML, "html", false, "also generate HTML review in temp directory")
}

func runFold(cmd *cobra.Command, args []string) error {
	backend := provider.Default()

	var session *parser.Session
	var err error

	if len(args) == 0 {
		session, err = selectSession(backend, foldAll)
	} else {
		projectName, sessionID := parseSessionArg(args[0])
		if foldProject != "" {
			projectName = foldProject
		}
		query, qErr := sessionLookupQuery(projectName, foldAll)
		if qErr != nil {
			return qErr
		}
		session, err = resolveSessionInQuery(backend, query, sessionID)
	}
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("no session found")
	}

	fullSession, err := backend.ParseSession(session.FilePath)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	result := fold.Analyze(fullSession)

	repoDir := findGitRoot()
	if repoDir != "" {
		if err := fold.CorrelateGit(result, repoDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: git correlation failed: %v\n", err)
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if foldOutput != "" {
		dir := filepath.Dir(foldOutput)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("mkdir: %w", err)
			}
		}
		if err := os.WriteFile(foldOutput, jsonBytes, 0644); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Fold written to: %s\n", foldOutput)
	} else {
		fmt.Println(string(jsonBytes))
	}

	if foldHTML {
		if err := generateFoldHTML(result, fullSession); err != nil {
			fmt.Fprintf(os.Stderr, "warning: HTML generation failed: %v\n", err)
		}
	}

	return nil
}

func generateFoldHTML(result *fold.FoldResult, session *parser.Session) error {
	tmpDir := os.TempDir()

	slug := session.Slug
	if slug == "" && len(session.ID) > 8 {
		slug = session.ID[:8]
	}
	filename := fmt.Sprintf("ccx-fold-%s-%s.html",
		time.Now().Format("20060102-150405"), slug)
	htmlPath := filepath.Join(tmpDir, filename)

	html := fold.RenderHTML(result)

	if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "HTML review: %s\n", htmlPath)
	openBrowser(htmlPath)
	return nil
}

func findGitRoot() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return filepath.Clean(string(out[:len(out)-1]))
}

// openBrowser is defined in web.go
