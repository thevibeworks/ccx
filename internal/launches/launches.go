// Package launches reads deva launch receipts and attributes sessions
// to goals by cwd + time. Receipts are advisory evidence: a missing
// dir, unreadable file, or malformed line is never an error.
package launches

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/thevibeworks/ccx/internal/config"
)

// Receipt is one launch stamp: an operator (via deva --goal) declaring
// that work in CWD from TS onward serves Goal.
type Receipt struct {
	TS        time.Time `json:"ts"`
	Goal      string    `json:"goal"`
	Agent     string    `json:"agent,omitempty"`
	CWD       string    `json:"cwd"`
	Container string    `json:"container,omitempty"`
	Source    string    `json:"source,omitempty"`
}

// A receipt stamps sessions in the same cwd that start within
// [TS-startGrace, TS+maxAge]; the latest qualifying receipt wins.
// startGrace absorbs receipt-vs-session clock ordering at launch;
// maxAge stops a stale stamp from claiming next week's work.
const (
	startGrace = 5 * time.Minute
	maxAge     = 24 * time.Hour
)

// Index answers "which goal was this session launched under?".
type Index struct {
	byCWD map[string][]Receipt // each slice sorted by TS ascending
}

// Dir is where deva writes receipts: <data>/launches/YYYY-MM-DD.jsonl.
func Dir() string {
	return filepath.Join(config.DataDir(), "launches")
}

// Load reads every receipt from the default dir.
func Load() *Index {
	return LoadDir(Dir())
}

// LoadDir reads every *.jsonl receipt file under dir.
func LoadDir(dir string) *Index {
	idx := &Index{byCWD: map[string][]Receipt{}}
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil || len(files) == 0 {
		return idx
	}
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			var r Receipt
			if json.Unmarshal(sc.Bytes(), &r) != nil {
				continue
			}
			if r.Goal == "" || r.CWD == "" || r.TS.IsZero() {
				continue
			}
			idx.byCWD[r.CWD] = append(idx.byCWD[r.CWD], r)
		}
		fh.Close()
	}
	for cwd, rs := range idx.byCWD {
		sort.Slice(rs, func(i, j int) bool { return rs[i].TS.Before(rs[j].TS) })
		idx.byCWD[cwd] = rs
	}
	return idx
}

// GoalFor returns the goal a session was launched under, or "".
func (x *Index) GoalFor(workspace string, start time.Time) string {
	if x == nil || len(x.byCWD) == 0 || workspace == "" || start.IsZero() {
		return ""
	}
	goal := ""
	for _, r := range x.byCWD[workspace] {
		if r.TS.After(start.Add(startGrace)) {
			break // sorted ascending: later receipts can't qualify either
		}
		if start.Sub(r.TS) <= maxAge {
			goal = r.Goal // latest qualifying wins
		}
	}
	return goal
}

// Empty reports whether the index holds no receipts.
func (x *Index) Empty() bool {
	return x == nil || len(x.byCWD) == 0
}
