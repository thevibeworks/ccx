package web

import (
	"fmt"
	"strings"
)

// SpinnerVerbs is a ccx-flavored gerund list drawn at random while
// loading indicators are on screen. The flavor leans into our actual
// code paths (gob cache, path canonicalization, JSONL parsing) next
// to nonsense and cooking verbs borrowed from the Claude Code TUI
// tradition. Pure decoration — no correlation with internal state.
//
// If you add entries, keep them short (≤14 chars), gerund form, and
// mix tone: don't let the list drift into all-serious or all-jokes.
var SpinnerVerbs = []string{
	// Self-referential — ccx internals and rituals
	"Ccxing",
	"Unspooling",
	"Canonicalizing",
	"Gobbing",
	"Rehydrating",
	"Indexing",
	"Rollout-ing",
	"Parsing",
	"Recounting",
	"Tallying",
	"Reconciling",
	"Checksumming",
	"Forking",
	// Cognition
	"Pondering",
	"Ruminating",
	"Mulling",
	"Cogitating",
	"Musing",
	"Puzzling",
	// Craft / code
	"Architecting",
	"Wrangling",
	"Orchestrating",
	"Hashing",
	"Stitching",
	"Triangulating",
	"Disambiguating",
	// Cooking (the canonical Claude Code flavor)
	"Sauteing",
	"Blanching",
	"Proofing",
	"Zesting",
	"Kneading",
	// Motion / dance
	"Gallivanting",
	"Skedaddling",
	"Moonwalking",
	// Nonsense (our Seussian quota)
	"Discombobulating",
	"Reticulating",
	"Flibbertigibbeting",
	"Whatchamacalliting",
}

// spinnerVerbsJSArray renders SpinnerVerbs as a JavaScript array
// literal safe to embed in a <script> block. Each entry is JSON-
// escaped so rogue characters in the list can't break the page.
func spinnerVerbsJSArray() string {
	var b strings.Builder
	b.WriteString("[")
	for i, v := range SpinnerVerbs {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(fmt.Sprintf("%q", v))
	}
	b.WriteString("]")
	return b.String()
}
