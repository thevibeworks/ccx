package cmd

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeSanitizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/tmp/foo", "-tmp-foo"},
		{"/Users/eric/src/project", "-Users-eric-src-project"},
		{"/home/user/my-project", "-home-user-my-project"},
		{"/tmp/test fork resume", "-tmp-test-fork-resume"},
		{"C:\\Users\\foo", "C--Users-foo"},
	}
	for _, tt := range tests {
		got := claudeSanitizePath(tt.input)
		if got != tt.want {
			t.Errorf("claudeSanitizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestClaudeSanitizePathKeepsLeadingDash(t *testing.T) {
	got := claudeSanitizePath("/tmp/foo")
	if !strings.HasPrefix(got, "-") {
		t.Errorf("should keep leading dash, got %q", got)
	}
}

func TestForkRewritesSessionIDAndCWD(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.jsonl")
	dstPath := filepath.Join(dir, "target.jsonl")

	lines := []string{
		`{"type":"user","sessionId":"old-id","cwd":"/old/path","uuid":"u1","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","sessionId":"old-id","cwd":"/old/path","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"hello"}}`,
		`{"type":"summary","summary":"test session","leafUuid":"a1"}`,
	}
	if err := os.WriteFile(srcPath, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := forkSession(srcPath, dstPath, "new-session-id", "/new/path")
	if err != nil {
		t.Fatalf("forkSession() error: %v", err)
	}
	if result.lineCount != 3 {
		t.Errorf("lineCount = %d, want 3", result.lineCount)
	}

	f, _ := os.Open(dstPath)
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var record map[string]any
		json.Unmarshal([]byte(scanner.Text()), &record)

		if sid, ok := record["sessionId"].(string); ok {
			if sid != "new-session-id" {
				t.Errorf("sessionId = %q, want new-session-id", sid)
			}
		}
		if cwd, ok := record["cwd"].(string); ok {
			if cwd != "/new/path" {
				t.Errorf("cwd = %q, want /new/path", cwd)
			}
		}
		if record["type"] == "summary" {
			if _, ok := record["sessionId"]; ok {
				t.Error("summary should not have sessionId added")
			}
		}
	}
}

func TestForkDropsFileHistorySnapshot(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.jsonl")
	dstPath := filepath.Join(dir, "target.jsonl")

	lines := []string{
		`{"type":"file-history-snapshot","messageId":"m1","snapshot":{}}`,
		`{"type":"user","sessionId":"old","cwd":"/old","uuid":"u1","message":{"role":"user","content":"hi"}}`,
		`{"type":"file-history-snapshot","messageId":"m2","snapshot":{},"isSnapshotUpdate":true}`,
	}
	os.WriteFile(srcPath, []byte(strings.Join(lines, "\n")+"\n"), 0644)

	result, err := forkSession(srcPath, dstPath, "new-id", "/new")
	if err != nil {
		t.Fatal(err)
	}
	if result.lineCount != 1 {
		t.Errorf("lineCount = %d, want 1 (file-history-snapshot should be dropped)", result.lineCount)
	}

	content, _ := os.ReadFile(dstPath)
	if strings.Contains(string(content), "file-history-snapshot") {
		t.Error("forked output should not contain file-history-snapshot records")
	}
}

func TestForkNullsWorktreeState(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.jsonl")
	dstPath := filepath.Join(dir, "target.jsonl")

	lines := []string{
		`{"type":"user","sessionId":"old","cwd":"/old","uuid":"u1","message":{"role":"user","content":"hi"}}`,
		`{"type":"worktree-state","worktreeSession":{"worktreePath":"/old/worktree"},"worktreePath":"/old/worktree"}`,
	}
	os.WriteFile(srcPath, []byte(strings.Join(lines, "\n")+"\n"), 0644)

	_, err := forkSession(srcPath, dstPath, "new-id", "/new")
	if err != nil {
		t.Fatal(err)
	}

	f, _ := os.Open(dstPath)
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var record map[string]any
		json.Unmarshal([]byte(scanner.Text()), &record)
		if record["type"] == "worktree-state" {
			if record["worktreeSession"] != nil {
				t.Error("worktreeSession should be null in forked output")
			}
			if record["worktreePath"] != nil {
				t.Error("worktreePath should be null in forked output")
			}
		}
	}
}

func TestForkPreservesParentUUIDChain(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.jsonl")
	dstPath := filepath.Join(dir, "target.jsonl")

	lines := []string{
		`{"type":"user","sessionId":"old","cwd":"/old","uuid":"u1","parentUuid":null,"message":{"role":"user","content":"q"}}`,
		`{"type":"assistant","sessionId":"old","cwd":"/old","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"a"}}`,
		`{"type":"user","sessionId":"old","cwd":"/old","uuid":"u2","parentUuid":"a1","message":{"role":"user","content":"q2"}}`,
	}
	os.WriteFile(srcPath, []byte(strings.Join(lines, "\n")+"\n"), 0644)

	forkSession(srcPath, dstPath, "new-id", "/new")

	f, _ := os.Open(dstPath)
	defer f.Close()
	scanner := bufio.NewScanner(f)
	uuids := []string{}
	parents := []any{}
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal([]byte(scanner.Text()), &record); err != nil {
			t.Fatalf("failed to unmarshal JSONL record: %v", err)
		}
		if u, ok := record["uuid"].(string); ok {
			uuids = append(uuids, u)
		}
		parents = append(parents, record["parentUuid"])
	}

	if uuids[0] != "u1" || uuids[1] != "a1" || uuids[2] != "u2" {
		t.Errorf("message UUIDs should be preserved, got %v", uuids)
	}
	if parents[1] != "u1" {
		t.Errorf("parentUuid chain broken: a1 parent = %v, want u1", parents[1])
	}
	if parents[2] != "a1" {
		t.Errorf("parentUuid chain broken: u2 parent = %v, want a1", parents[2])
	}
}
