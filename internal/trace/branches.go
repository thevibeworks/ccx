package trace

import "github.com/thevibeworks/ccx/internal/parser"

// A user edit or resend appears in the log as two user records sharing
// one parentUuid: a branch point, not two turns. Counting both inflates
// turn_count with a phantom turn and double-reports the prompt. The
// abandoned sibling is still evidence (the human changed course), so it
// is marked superseded rather than dropped.

// detectSupersededAnchors maps each turn-anchor UUID abandoned by a
// branch to the UUID of the sibling that replaced it. The last sibling
// wins: session logs are append-only, so a resend is always recorded
// after the branch it supersedes. Follow-up prompts inside an abandoned
// subtree are superseded by the same replacement as the subtree root.
func detectSupersededAnchors(messages []*parser.Message) map[string]string {
	byUUID := make(map[string]*parser.Message, len(messages))
	for _, msg := range messages {
		if msg != nil && msg.UUID != "" {
			byUUID[msg.UUID] = msg
		}
	}

	siblings := make(map[string][]*parser.Message)
	for _, msg := range messages {
		if !isTurnAnchor(msg) || msg.UUID == "" || msg.ParentUUID == "" {
			continue
		}
		// A parent absent from the log means truncation, not branching.
		if _, ok := byUUID[msg.ParentUUID]; !ok {
			continue
		}
		siblings[msg.ParentUUID] = append(siblings[msg.ParentUUID], msg)
	}

	superseded := make(map[string]string)
	var abandoned []*parser.Message
	var replacedBy []string
	for _, group := range siblings {
		if len(group) < 2 {
			continue
		}
		winner := group[len(group)-1]
		for _, loser := range group[:len(group)-1] {
			superseded[loser.UUID] = winner.UUID
			abandoned = append(abandoned, loser)
			replacedBy = append(replacedBy, winner.UUID)
		}
	}
	for i, loser := range abandoned {
		markAbandonedSubtree(loser, replacedBy[i], superseded)
	}
	return superseded
}

// markAbandonedSubtree records every turn anchor under an abandoned
// branch root as superseded by the branch's replacement. Anchors that
// already carry a nearer replacement (a branch inside the abandoned
// subtree) keep it.
func markAbandonedSubtree(msg *parser.Message, winner string, superseded map[string]string) {
	for _, child := range msg.Children {
		if child == nil {
			continue
		}
		if isTurnAnchor(child) && child.UUID != "" {
			if _, done := superseded[child.UUID]; !done {
				superseded[child.UUID] = winner
			}
		}
		markAbandonedSubtree(child, winner, superseded)
	}
}

// isTurnAnchor matches the messages segmentTurns opens turns on.
func isTurnAnchor(msg *parser.Message) bool {
	return msg != nil && !msg.IsSidechain &&
		(msg.Kind == parser.KindUserPrompt || msg.Kind == parser.KindCommand)
}

// markSupersededTurns flags turns whose anchor was abandoned by a
// branch and resolves the replacement to its turn index. Returns the
// number of turns marked.
func markSupersededTurns(turns []Turn, superseded map[string]string) int {
	if len(superseded) == 0 {
		return 0
	}
	indexByAnchor := make(map[string]int, len(turns))
	for _, t := range turns {
		indexByAnchor[t.AnchorID] = t.Index
	}
	count := 0
	for i := range turns {
		winner, ok := superseded[turns[i].AnchorID]
		if !ok {
			continue
		}
		turns[i].Superseded = true
		turns[i].SupersededByTurn = indexByAnchor[winner]
		count++
	}
	return count
}
