package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"regexp"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/provider"
)

var forkCmd = &cobra.Command{
	Use:   "fork <session-id>",
	Short: "Fork a session to the current project for resuming",
	Long: `Copy a session from any project into the current project directory,
rewriting the session ID and CWD so you can resume it with 'claude --resume'.

The original session is never modified. The forked copy gets a new UUID
and points to the target directory.

Examples:
  ccx fork abc12345              # Fork to current directory
  ccx fork abc12345 --to /path   # Fork to specific directory`,
	Args: cobra.ExactArgs(1),
	RunE: runFork,
}

var forkTo string

func init() {
	forkCmd.Flags().StringVar(&forkTo, "to", "", "target directory (default: current working directory)")
	rootCmd.AddCommand(forkCmd)
}

func runFork(cmd *cobra.Command, args []string) error {
	sessionQuery := args[0]
	backend := provider.Default()

	session, err := backend.FindSession("", sessionQuery)
	if err != nil {
		return fmt.Errorf("failed to search sessions: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session not found: %s", sessionQuery)
	}

	if session.Provider == "codex" {
		return fmt.Errorf("fork only supports Claude Code sessions (codex uses a different format)")
	}

	targetDir := forkTo
	if targetDir == "" {
		targetDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}
	targetDir, err = filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("invalid target directory: %w", err)
	}
	// Canonicalize via EvalSymlinks to match Claude Code's realpath resolution
	if resolved, err := filepath.EvalSymlinks(targetDir); err == nil {
		targetDir = resolved
	}

	settings := config.Load()
	encodedTarget := claudeSanitizePath(targetDir)
	targetProjectDir := filepath.Join(settings.ClaudeHome, "projects", encodedTarget)
	if err := os.MkdirAll(targetProjectDir, 0755); err != nil {
		return fmt.Errorf("failed to create target project directory: %w", err)
	}

	newSessionID := uuid.New().String()
	targetPath := filepath.Join(targetProjectDir, newSessionID+".jsonl")

	result, err := forkSession(session.FilePath, targetPath, newSessionID, targetDir)
	if err != nil {
		return err
	}
	lineCount := result.lineCount

	srcID := session.ID
	if len(srcID) > 8 {
		srcID = srcID[:8]
	}
	newIDShort := newSessionID[:8]

	fmt.Printf("Forked session %s -> %s (%d lines)\n", srcID, newIDShort, lineCount)
	fmt.Printf("  Source: %s\n", session.FilePath)
	fmt.Printf("  Target: %s\n", targetPath)
	fmt.Println()
	fmt.Println("Resume with:")
	fmt.Printf("  cd %s && claude --resume %s\n", targetDir, newSessionID)

	return nil
}

type forkResult struct {
	lineCount int
}

func forkSession(srcPath, dstPath, newSessionID, targetDir string) (*forkResult, error) {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source session: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create target session: %w", err)
	}
	defer dstFile.Close()

	writer := bufio.NewWriter(dstFile)
	scanner := bufio.NewScanner(srcFile)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			_, _ = writer.WriteString(line)
			_ = writer.WriteByte('\n')
			lineCount++
			continue
		}

		recordType, _ := record["type"].(string)

		if recordType == "file-history-snapshot" {
			continue
		}

		if recordType == "worktree-state" {
			record["worktreeSession"] = nil
			record["worktreePath"] = nil
		}

		if _, ok := record["sessionId"]; ok {
			record["sessionId"] = newSessionID
		}
		if _, ok := record["cwd"]; ok {
			record["cwd"] = targetDir
		}

		rewritten, err := json.Marshal(record)
		if err != nil {
			_, _ = writer.WriteString(line)
			_ = writer.WriteByte('\n')
			lineCount++
			continue
		}

		_, _ = writer.Write(rewritten)
		_ = writer.WriteByte('\n')
		lineCount++
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading source session: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return nil, fmt.Errorf("error writing target session: %w", err)
	}

	return &forkResult{lineCount: lineCount}, nil
}

var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]`)

// claudeSanitizePath matches Claude Code's sanitizePath exactly:
// replace all non-alphanumeric chars with '-', no stripping.
// /tmp/foo → -tmp-foo (NOT tmp-foo like parser.EncodePath does)
func claudeSanitizePath(path string) string {
	return nonAlphanumeric.ReplaceAllString(path, "-")
}
