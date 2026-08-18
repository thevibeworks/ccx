package render

import (
	"fmt"
	"strings"

	"github.com/thevibeworks/ccx/internal/parser"
)

// WindowSession returns a copy of session holding only the message
// whose UUID starts with target plus `before` messages before it and
// `after` after it, in wire order, flattened (children detached so
// the window is exactly what it says). This is the walk from a
// citation — a search hit's message_id, a trace step's message_id —
// back to its surrounding context without leaving ccx. index/total
// locate the target in the full session for the caller's header.
func WindowSession(session *parser.Session, target string, before, after int) (*parser.Session, int, int, error) {
	if session == nil {
		return nil, 0, 0, fmt.Errorf("no session")
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, 0, 0, fmt.Errorf("--at needs a message id (or unique prefix)")
	}
	all := parser.FlattenSessionMessages(session)
	idx := -1
	for i, m := range all {
		if m == nil || !strings.HasPrefix(m.UUID, target) {
			continue
		}
		if m.UUID == target {
			idx = i
			break
		}
		if idx >= 0 && all[idx].UUID != m.UUID {
			return nil, 0, 0, fmt.Errorf("message id prefix %q is ambiguous (%s, %s, ...); use more characters", target, all[idx].UUID, m.UUID)
		}
		idx = i
	}
	if idx < 0 {
		return nil, 0, 0, fmt.Errorf("no message with id prefix %q in session %s", target, session.ID)
	}
	if before < 0 {
		before = 0
	}
	if after < 0 {
		after = 0
	}
	start := idx - before
	if start < 0 {
		start = 0
	}
	end := idx + after + 1
	if end > len(all) {
		end = len(all)
	}
	out := *session
	out.RootMessages = make([]*parser.Message, 0, end-start)
	for _, m := range all[start:end] {
		flat := *m
		flat.Children = nil
		out.RootMessages = append(out.RootMessages, &flat)
	}
	return &out, idx + 1, len(all), nil
}
