package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Prompt history as a search source. Claude Code and Codex append every
// human prompt to a history file that outlives session cleanup, so it
// is the longest-lived evidence for "when did we first say X". Only
// prompts whose session is no longer in the store surface here — when
// the session file exists its own content hit is the better citation,
// and one prompt must not appear twice in a timeline.

// promptHit is one matching prompt from a history file.
type promptHit struct {
	Provider  string
	Project   string // workspace basename when the file records one
	SessionID string
	Path      string
	Line      int
	Time      time.Time
	Matches   int
	Quote     string
}

// promptHistoryFiles lists the history files to scan for the given
// provider homes (missing files are simply absent).
func promptHistoryFiles(claudeHome, codexHome string) map[string]string {
	files := map[string]string{}
	if claudeHome != "" {
		files["claude-code"] = filepath.Join(claudeHome, "history.jsonl")
	}
	if codexHome != "" {
		files["codex"] = filepath.Join(codexHome, "history.jsonl")
	}
	return files
}

// scanPromptHistory returns matching prompts across the history files,
// skipping entries whose session id is in knownSessions.
func scanPromptHistory(files map[string]string, m textMatcher, knownSessions map[string]bool) []promptHit {
	var hits []promptHit
	for _, provider := range []string{"claude-code", "codex"} {
		path, ok := files[provider]
		if !ok {
			continue
		}
		hits = append(hits, scanPromptHistoryFile(provider, path, m, knownSessions)...)
	}
	return hits
}

func scanPromptHistoryFile(provider, path string, m textMatcher, knownSessions map[string]bool) []promptHit {
	file, err := os.Open(path)
	if err != nil {
		return nil // no history is normal
	}
	defer file.Close()

	var hits []promptHit
	reader := bufio.NewReaderSize(file, 64*1024)
	lineNo := 0
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			lineNo++
			// Cheap literal prefilter on the raw line; the decoded
			// text decides.
			if rawPrefilterSafe(m.query) && !m.literal().matches(line) {
				goto next
			}
			if hit, ok := decodePromptLine(provider, line); ok {
				if hit.SessionID != "" && knownSessions[hit.SessionID] {
					goto next
				}
				if n := m.count(hit.text); n > 0 {
					idx, qlen := m.index(hit.text)
					hits = append(hits, promptHit{
						Provider:  provider,
						Project:   hit.project,
						SessionID: hit.SessionID,
						Path:      path,
						Line:      lineNo,
						Time:      hit.time,
						Matches:   n,
						Quote:     matchSnippet(hit.text, idx, qlen),
					})
				}
			}
		}
	next:
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "warning: read error in %s: %v\n", filepath.Base(path), err)
			}
			return hits
		}
	}
}

type promptLine struct {
	SessionID string
	project   string
	time      time.Time
	text      string
}

// decodePromptLine understands both history formats:
//
//	claude-code: {"display":"...","project":"/path","sessionId":"...","timestamp":<ms>}
//	codex:       {"session_id":"...","ts":<s>,"text":"..."}
func decodePromptLine(provider, line string) (promptLine, bool) {
	switch provider {
	case "claude-code":
		var rec struct {
			Display   string `json:"display"`
			Project   string `json:"project"`
			SessionID string `json:"sessionId"`
			Timestamp int64  `json:"timestamp"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil || rec.Display == "" {
			return promptLine{}, false
		}
		out := promptLine{SessionID: rec.SessionID, text: rec.Display}
		if rec.Project != "" {
			out.project = filepath.Base(rec.Project)
		}
		if rec.Timestamp > 0 {
			out.time = time.UnixMilli(rec.Timestamp).UTC()
		}
		return out, true
	case "codex":
		var rec struct {
			SessionID string `json:"session_id"`
			TS        int64  `json:"ts"`
			Text      string `json:"text"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil || rec.Text == "" {
			return promptLine{}, false
		}
		out := promptLine{SessionID: rec.SessionID, text: rec.Text}
		if rec.TS > 0 {
			out.time = time.Unix(rec.TS, 0).UTC()
		}
		return out, true
	}
	return promptLine{}, false
}

// promptResult renders a prompt hit as a search result row.
func promptResult(h promptHit) searchResult {
	project := h.Project
	if project == "" {
		project = "(" + h.Provider + " history)"
	}
	session := ""
	if h.SessionID != "" {
		session = truncateID(h.SessionID, 8)
	}
	res := searchResult{
		Type:     "prompt",
		Project:  project,
		Session:  session,
		Path:     h.Path,
		Summary:  fmt.Sprintf("%d hits · [user] %s", h.Matches, truncateDisplay(h.Quote, 56)),
		Time:     "-",
		Matches:  h.Matches,
		Previews: []contentPreview{{Role: "user", Text: h.Quote}},
		Priority: 4,
		firstHit: h.Time,
		hits: []searchHit{{
			Project: project, Session: session, Path: h.Path, MessageID: fmt.Sprintf("line:%d", h.Line),
			Time: h.Time, Role: "user", Matches: h.Matches, Quote: h.Quote,
		}},
	}
	if !h.Time.IsZero() {
		res.FirstHit = h.Time.UTC().Format(time.RFC3339)
	}
	return res
}

// knownSessionIDs collects every session id the store currently holds.
func knownSessionIDs(sessions []sessionCandidate) map[string]bool {
	known := make(map[string]bool, len(sessions))
	for _, c := range sessions {
		if c.session != nil && c.session.ID != "" {
			known[strings.ToLower(c.session.ID)] = true
		}
	}
	return known
}
