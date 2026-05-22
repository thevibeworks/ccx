package fold

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func CorrelateGit(result *TraceResult, repoDir string) error {
	if result == nil || repoDir == "" {
		return nil
	}
	exchanges := result.Exchanges

	result.Git.RepoRoot = repoDir
	result.Git.Branch = gitOutput(repoDir, "branch", "--show-current")
	result.Git.Head = gitOutput(repoDir, "rev-parse", "HEAD")
	sessionRoot := result.Session.CWD
	if sessionRoot == "" {
		sessionRoot = repoDir
	}

	status, err := gitStatus(repoDir)
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	result.Git.UncommittedFiles = status
	result.Git.Dirty = len(status) > 0
	result.Git.UncommittedStat = gitOutput(repoDir, "diff", "--stat", "HEAD", "--")
	result.Stats.UncommittedFiles = len(status)

	if len(exchanges) == 0 {
		return nil
	}

	start := result.Session.Start
	end := result.Session.End
	if start.IsZero() || end.IsZero() {
		result.Warnings = append(result.Warnings, TraceWarning{
			Kind:    "git_time_window_missing",
			Message: "session start or end timestamp missing; skipped commit correlation",
		})
		return nil
	}

	commits, err := listCommits(repoDir, start, end)
	if err != nil {
		return fmt.Errorf("git log: %w", err)
	}
	if len(commits) == 0 {
		result.Warnings = append(result.Warnings, TraceWarning{
			Kind:    "git_no_commits",
			Message: "no commits found in the session time window",
		})
		return nil
	}

	for i := range commits {
		files, err := commitFiles(repoDir, commits[i].SHA)
		if err == nil {
			commits[i].Files = files
		}
	}

	result.Git.Commits = commits

	editedByExchange := make(map[int]map[string]struct{})
	for i, exchange := range exchanges {
		m := make(map[string]struct{})
		for _, f := range exchange.FilesEdited {
			for _, normalized := range normalizeEvidencePath(repoDir, sessionRoot, f) {
				m[normalized] = struct{}{}
			}
		}
		editedByExchange[i] = m
	}

	var links []ExchangeCommitLink
	linkedCommits := make(map[string]struct{})

	for _, commit := range commits {
		bestExchange := -1
		var bestOverlap []string

		for i := range exchanges {
			edited := editedByExchange[i]
			if len(edited) == 0 {
				continue
			}
			var overlap []string
			for _, cf := range commit.Files {
				if _, ok := edited[cf]; ok {
					overlap = append(overlap, cf)
				}
			}
			if len(overlap) > len(bestOverlap) {
				bestOverlap = overlap
				bestExchange = i
			}
		}

		if bestExchange >= 0 && len(bestOverlap) > 0 {
			links = append(links, ExchangeCommitLink{
				ExchangeIndex: exchanges[bestExchange].Index,
				CommitSHA:     commit.SHA,
				FileOverlap:   bestOverlap,
				Confidence:    "high",
			})
			linkedCommits[commit.SHA] = struct{}{}

			if len(result.Exchanges) > bestExchange {
				result.Exchanges[bestExchange].LinkedCommits = append(
					result.Exchanges[bestExchange].LinkedCommits, commit.SHA)
			}
		}
	}

	result.Git.ExchangeCommitLinks = links
	result.Stats.CommitsLinked = len(linkedCommits)
	return nil
}

func listCommits(repoDir string, after, before time.Time) ([]GitCommit, error) {
	afterStr := after.Add(-1 * time.Minute).Format(time.RFC3339)
	beforeStr := before.Add(1 * time.Minute).Format(time.RFC3339)

	cmd := exec.Command("git", "log",
		"--after="+afterStr,
		"--before="+beforeStr,
		"--format=%H\t%aI\t%s",
		"--all",
	)
	cmd.Dir = repoDir

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var commits []GitCommit
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		commits = append(commits, GitCommit{
			SHA:       parts[0],
			Timestamp: parts[1],
			Subject:   parts[2],
		})
	}
	return commits, nil
}

func commitFiles(repoDir, sha string) ([]string, error) {
	cmd := exec.Command("git", "diff-tree", "--no-commit-id", "--name-only", "-r", sha)
	cmd.Dir = repoDir

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var files []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		f := strings.TrimSpace(scanner.Text())
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

func normalizeEvidencePath(repoRoot, sessionCWD, path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	path = filepath.Clean(path)
	var candidates []string

	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(repoRoot, path); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			candidates = append(candidates, filepath.ToSlash(rel))
		}
	} else {
		candidates = append(candidates, filepath.ToSlash(strings.TrimPrefix(path, "./")))
		if sessionCWD != "" {
			abs := filepath.Join(sessionCWD, path)
			if rel, err := filepath.Rel(repoRoot, abs); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
				candidates = append(candidates, filepath.ToSlash(rel))
			}
		}
	}

	seen := make(map[string]struct{})
	var out []string
	for _, candidate := range candidates {
		candidate = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(candidate)), "./")
		if candidate == "." || candidate == "" || strings.HasPrefix(candidate, "../") {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	sort.Strings(out)
	return out
}

func gitStatus(repoDir string) ([]GitFileStatus, error) {
	cmd := exec.Command("git", "status", "--short")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []GitFileStatus
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 4 {
			continue
		}
		files = append(files, GitFileStatus{
			Status: strings.TrimSpace(line[:2]),
			Path:   strings.TrimSpace(line[3:]),
		})
	}
	return files, nil
}

func gitOutput(repoDir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func parseTimestamp(ts string) time.Time {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}
	}
	return t
}
