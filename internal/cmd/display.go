package cmd

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func sessionSummaryPreview(summary string, max int) string {
	preview := cleanDisplayText(summary)
	preview = strings.Trim(preview, "`'\" ")
	if preview == "" {
		preview = "(no summary)"
	}
	return truncateDisplay(preview, max)
}

func cleanDisplayText(s string) string {
	s = stripANSI(s)
	var out strings.Builder
	out.Grow(len(s))

	lastSpace := false
	for _, r := range s {
		if r == '\uFFFD' {
			continue
		}
		if r == '\t' || r == '\n' || r == '\r' || unicode.IsSpace(r) {
			if !lastSpace && out.Len() > 0 {
				out.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		out.WriteRune(r)
		lastSpace = false
	}

	return strings.TrimSpace(out.String())
}

func stripANSI(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			out.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) || s[i] != '[' {
			continue
		}
		i++
		for i < len(s) {
			b := s[i]
			if b >= 0x40 && b <= 0x7e {
				break
			}
			i++
		}
	}
	return out.String()
}

func truncateDisplay(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if displayLen(s) <= max {
		return s
	}
	if max <= 3 {
		return strings.Repeat(".", max)
	}

	limit := max - 3
	var out strings.Builder
	width := 0
	for _, r := range s {
		w := runeDisplayWidth(r)
		if width+w > limit {
			break
		}
		out.WriteRune(r)
		width += w
	}
	return strings.TrimRight(out.String(), " ") + "..."
}

func displayLen(s string) int {
	n := 0
	for _, r := range s {
		n += runeDisplayWidth(r)
	}
	return n
}

func runeDisplayWidth(r rune) int {
	if r == 0 || unicode.IsControl(r) {
		return 0
	}
	if utf8.RuneLen(r) > 1 && !unicode.In(r, unicode.Latin, unicode.Common) {
		return 2
	}
	return 1
}
