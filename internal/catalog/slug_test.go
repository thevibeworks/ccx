package catalog

import (
	"testing"

	"github.com/thevibeworks/ccx/internal/parser"
)

func TestProjectMatchesNameSlugNormalized(t *testing.T) {
	project := &parser.Project{
		Name: "260715-ccx-session-watch",
		Path: "/wrk/WIP/260715_ccx-session-watch",
	}
	if !ProjectMatchesName(project, "260715_ccx-session-watch") {
		t.Fatal("underscore directory name should match dash slug")
	}
	if !ProjectMatchesName(project, "WIP/260715-ccx-session-watch") {
		t.Fatal("dash query should match underscore path via slug fold")
	}
	if ProjectMatchesName(project, "totally-different") {
		t.Fatal("unrelated query must not match")
	}
}

func TestSlugifyProjectQuery(t *testing.T) {
	cases := map[string]string{
		"260715_ccx-session-watch": "260715-ccx-session-watch",
		"/Wrk/WIP/Some_Project":    "wrk-wip-some-project",
		"  spaced   out  ":         "spaced-out",
		"///":                      "",
		"":                         "",
	}
	for in, want := range cases {
		if got := SlugifyProjectQuery(in); got != want {
			t.Fatalf("SlugifyProjectQuery(%q) = %q, want %q", in, got, want)
		}
	}
}
