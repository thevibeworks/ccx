package i18n

import (
	"net/http"
	"strconv"
	"strings"
)

// Detect picks a locale for an HTTP request.
//
// Precedence (most specific first):
//  1. ?lang= query — explicit user action, also persisted as a cookie
//  2. ccx-lang cookie — last explicit choice
//  3. configured locale, when it is a real catalog (not auto/empty)
//  4. Accept-Language
//  5. English
//
// fromQuery is true only when the query param selected the locale,
// so the caller can persist the cookie.
func Detect(r *http.Request, configured string) (t *Translator, fromQuery bool) {
	if r != nil {
		if raw := strings.TrimSpace(r.URL.Query().Get(QueryParam)); raw != "" {
			if code := Normalize(raw); code != "" {
				return For(code), true
			}
		}
		if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
			if code := Normalize(c.Value); code != "" {
				return For(code), false
			}
		}
	}
	if code := Normalize(configured); code != "" {
		return For(code), false
	}
	if r != nil {
		if code := matchAcceptLanguage(r.Header.Get("Accept-Language")); code != "" {
			return For(code), false
		}
	}
	return Default(), false
}

// matchAcceptLanguage walks a standard Accept-Language header and
// returns the first tag we have a catalog for. q-values are honored
// so "en;q=0.8, zh-CN;q=0.9" selects zh-CN.
func matchAcceptLanguage(header string) string {
	if header == "" {
		return ""
	}
	type tagged struct {
		code string
		q    float64
	}
	var tags []tagged
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, rest, _ := strings.Cut(part, ";")
		q := 1.0
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "q=") {
			if v, err := strconv.ParseFloat(strings.TrimSpace(rest[2:]), 64); err == nil {
				q = v
			}
		}
		if q <= 0 {
			continue // q=0 means "not acceptable"
		}
		code := Normalize(strings.TrimSpace(name))
		if code == "" {
			// Prefix match: zh-HK → no Traditional catalog, skip.
			// zh → already handled by Normalize.
			continue
		}
		tags = append(tags, tagged{code: code, q: q})
	}
	bestQ := -1.0
	best := ""
	for _, t := range tags {
		if t.q > bestQ {
			bestQ = t.q
			best = t.code
		}
	}
	return best
}

// Cookie builds the persistence cookie for an explicit language choice.
func Cookie(code string) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    Normalize(code),
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: false, // language switcher is a <select> in the page
	}
}
