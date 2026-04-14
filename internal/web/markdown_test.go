package web

import (
	"strings"
	"testing"
)

func TestParseATXHeading_AllLevels(t *testing.T) {
	cases := []struct {
		in    string
		level int
		text  string
	}{
		{"# Heading", 1, "Heading"},
		{"## Heading", 2, "Heading"},
		{"### Heading", 3, "Heading"},
		{"#### Heading", 4, "Heading"},
		{"##### Heading", 5, "Heading"},
		{"###### Heading", 6, "Heading"},
		{"## With trailing  ##", 2, "With trailing"},
		{"## Heading with **bold**", 2, "Heading with **bold**"},
	}
	for _, c := range cases {
		level, text, ok := parseATXHeading(c.in)
		if !ok {
			t.Errorf("parseATXHeading(%q) returned ok=false", c.in)
			continue
		}
		if level != c.level {
			t.Errorf("parseATXHeading(%q) level = %d, want %d", c.in, level, c.level)
		}
		if text != c.text {
			t.Errorf("parseATXHeading(%q) text = %q, want %q", c.in, text, c.text)
		}
	}
}

func TestParseATXHeading_NotHeadings(t *testing.T) {
	notHeadings := []string{
		"Just a paragraph",
		"####### Too many hashes",
		"##NoSpace",
		"## ",     // no content (just trailing whitespace)
		"##",      // no content at all
		"#",       // single # with no content
		"",        // empty line
		" ## leading space", // indented — not a heading per CommonMark
	}
	for _, line := range notHeadings {
		if _, _, ok := parseATXHeading(line); ok {
			t.Errorf("parseATXHeading(%q) returned ok=true, expected false", line)
		}
	}
}

func TestRenderMarkdown_RendersH2Heading(t *testing.T) {
	// Server-side renderMarkdown must emit a div.md-h2 when the input
	// starts with "## ". Before this fix, the line was wrapped in <p>.
	out := renderMarkdown("## Summary\nSome text below")
	if !strings.Contains(out, `class="md-h2"`) {
		t.Errorf("renderMarkdown should emit md-h2 for '## Summary', got: %s", out)
	}
	if !strings.Contains(out, `>Summary<`) {
		t.Errorf("renderMarkdown should include the heading text, got: %s", out)
	}
	// And the line must NOT be wrapped as a paragraph containing the ## literal
	if strings.Contains(out, `<p>## Summary</p>`) {
		t.Error("renderMarkdown should NOT wrap heading line as literal paragraph")
	}
}

func TestRenderMarkdown_RendersAllHeadingLevels(t *testing.T) {
	input := "# H1\n## H2\n### H3\n#### H4\n##### H5\n###### H6"
	out := renderMarkdown(input)
	for level := 1; level <= 6; level++ {
		want := "md-h" + string(rune('0'+level))
		if !strings.Contains(out, want) {
			t.Errorf("expected %s class in output, got: %s", want, out)
		}
	}
}

func TestRenderMarkdown_HeadingsKeepBoldAndCode(t *testing.T) {
	out := renderMarkdown("## Heading with **bold** and `code`")
	if !strings.Contains(out, `<strong>bold</strong>`) {
		t.Errorf("expected bold processing in heading, got: %s", out)
	}
	if !strings.Contains(out, `<code>code</code>`) {
		t.Errorf("expected inline code processing in heading, got: %s", out)
	}
}

func TestRenderMarkdown_RendersList(t *testing.T) {
	out := renderMarkdown("- first item\n- second item")
	if !strings.Contains(out, `class="md-li"`) {
		t.Errorf("expected md-li class in list rendering, got: %s", out)
	}
	if !strings.Contains(out, `first item`) || !strings.Contains(out, `second item`) {
		t.Errorf("expected list items in output, got: %s", out)
	}
}
