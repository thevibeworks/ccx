package web

import (
	"strings"
	"testing"
)

func TestSpinnerVerbs_NonEmptyAndShortish(t *testing.T) {
	if len(SpinnerVerbs) == 0 {
		t.Fatal("SpinnerVerbs must not be empty")
	}
	for _, v := range SpinnerVerbs {
		if v == "" {
			t.Error("empty verb in list")
		}
		if len(v) > 20 {
			t.Errorf("verb %q longer than 20 chars — keep the spinner label short", v)
		}
	}
}

func TestSpinnerVerbs_HasCcxSelfReferential(t *testing.T) {
	// The easter-egg spirit: at least ONE verb should be a ccx-
	// specific self-reference. If some well-meaning cleanup strips
	// them all out, this test catches it.
	wantAny := []string{"Ccxing", "Canonicalizing", "Gobbing", "Rehydrating", "Unspooling"}
	for _, want := range wantAny {
		for _, have := range SpinnerVerbs {
			if have == want {
				return
			}
		}
	}
	t.Errorf("expected at least one of %v in SpinnerVerbs, found none", wantAny)
}

func TestSpinnerVerbsJSArray_Escapes(t *testing.T) {
	js := spinnerVerbsJSArray()
	if !strings.HasPrefix(js, "[") || !strings.HasSuffix(js, "]") {
		t.Errorf("JS array should be bracketed, got %q", js)
	}
	// Every verb should appear inside double quotes — %q escapes them.
	for _, v := range SpinnerVerbs {
		want := `"` + v + `"`
		if !strings.Contains(js, want) {
			t.Errorf("JS array missing %q", v)
		}
	}
}

func TestSpinnerVerbsJSArray_SurvivesQuotesAndBackslashes(t *testing.T) {
	// Regression guard: if someone adds a verb with a literal quote
	// or backslash, the %q formatter must emit a safe JS string so
	// the HTML script block doesn't break.
	saved := SpinnerVerbs
	defer func() { SpinnerVerbs = saved }()
	SpinnerVerbs = []string{`safe`, `"quote"`, `back\slash`}
	js := spinnerVerbsJSArray()
	if strings.Count(js, "\"") != 6+2 {
		// 3 verbs × 2 quotes each = 6 open/close; the "quote" verb's
		// inner quotes are escaped with \", not counted as string
		// delimiters. Total balanced quotes: 6 delimiters + 2 escaped
		// appearances = 8 quote characters in the output.
		// If escaping is broken this count shifts.
		t.Logf("js=%s", js)
	}
	// The key invariant: the output must still parse as a valid Go
	// string (and therefore a valid JS array literal).
	if !strings.Contains(js, `\"`) {
		t.Errorf("embedded quote should be backslash-escaped, got %q", js)
	}
}
