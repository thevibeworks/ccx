package fold

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func CorrelateGit(result *FoldResult, repoDir string) error {
	if result == nil || len(result.Turns) == 0 {
		return nil
	}

	start := result.Session.Start
	end := result.Session.End
	if start.IsZero() || end.IsZero() {
		return nil
	}

	commits, err := listCommits(repoDir, start, end)
	if err != nil {
		return fmt.Errorf("git log: %w", err)
	}
	if len(commits) == 0 {
		return nil
	}

	for i := range commits {
		files, err := commitFiles(repoDir, commits[i].SHA)
		if err == nil {
			commits[i].Files = files
		}
	}

	result.Git.Commits = commits

	editedByTurn := make(map[int]map[string]struct{})
	for i, t := range result.Turns {
		m := make(map[string]struct{})
		for _, f := range t.FilesEdited {
			m[f] = struct{}{}
		}
		editedByTurn[i] = m
	}

	var links []TurnCommitLink
	linkedCommits := make(map[string]struct{})

	for _, commit := range commits {
		bestTurn := -1
		var bestOverlap []string

		for i, t := range result.Turns {
			edited := editedByTurn[i]
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
				bestTurn = i
			}
			if len(overlap) == 0 && t.End.Before(parseTimestamp(commit.Timestamp)) &&
				(bestTurn == -1 || len(bestOverlap) == 0) {
				bestTurn = i
			}
		}

		if bestTurn >= 0 {
			links = append(links, TurnCommitLink{
				TurnIndex:   result.Turns[bestTurn].Index,
				CommitSHA:   commit.SHA,
				FileOverlap: bestOverlap,
			})
			linkedCommits[commit.SHA] = struct{}{}

			result.Turns[bestTurn].LinkedCommits = append(
				result.Turns[bestTurn].LinkedCommits, commit.SHA)
		}
	}

	result.Git.TurnCommitLinks = links
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

func parseTimestamp(ts string) time.Time {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}
	}
	return t
}
