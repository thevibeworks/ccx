package web

import (
	"encoding/json"
	"html"
	"net/http"
	"strings"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/i18n"
)

// loc is the request-scoped translator used by HTML renderers.
// A zero value speaks English so tests that call render functions
// directly keep their existing assertions.
type loc struct {
	tr *i18n.Translator
}

func defaultLoc() loc {
	return loc{tr: i18n.Default()}
}

func locFrom(r *http.Request) loc {
	// Detect from the request itself so handler-level tests (no
	// middleware) still honor ?lang=, the cookie, and Accept-Language.
	tr, _ := i18n.Detect(r, config.Locale())
	return loc{tr: tr}
}

func (l loc) T(key string, args ...any) string {
	if l.tr == nil {
		return i18n.Default().T(key, args...)
	}
	return l.tr.T(key, args...)
}

func (l loc) Code() string {
	if l.tr == nil {
		return i18n.En
	}
	return l.tr.Code()
}

// localeMiddleware persists an explicit ?lang= choice as a cookie.
// Pages resolve their language with locFrom, so detection has one path.
func localeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if code := i18n.Normalize(r.URL.Query().Get(i18n.QueryParam)); code != "" {
			http.SetCookie(w, i18n.Cookie(code))
		}
		next.ServeHTTP(w, r)
	})
}

// jsKeyPrefix marks the catalog keys client JS reads through t().
// Only those are inlined, so a page does not carry the whole catalog.
const jsKeyPrefix = "js."

// i18nScript emits window.CCX_I18N / CCX_LOCALE / t() for client JS.
func (l loc) i18nScript() string {
	code := l.Code()
	cat := i18n.Catalog(code)
	if cat == nil {
		cat = i18n.Catalog(i18n.En)
	}
	for k := range cat {
		if !strings.HasPrefix(k, jsKeyPrefix) {
			delete(cat, k)
		}
	}
	raw, err := json.Marshal(cat)
	if err != nil {
		raw = []byte("{}")
	}
	var b strings.Builder
	b.WriteString(`<script>window.CCX_LOCALE=`)
	b.WriteString(strconvQuote(code))
	b.WriteString(`;window.CCX_I18N=`)
	b.Write(raw)
	b.WriteString(`;function t(key){var d=window.CCX_I18N||{},s=d[key];if(s==null)s=key;if(arguments.length<2)return s;var a=Array.prototype.slice.call(arguments,1),i=0;return String(s).replace(/%(?:%|s|d)/g,function(m){if(m==='%%')return '%';if(i>=a.length)return m;return String(a[i++]);});}</script>`)
	return b.String()
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (l loc) langSwitcher() string {
	label := html.EscapeString(l.T("nav.language"))
	var b strings.Builder
	b.WriteString(`<label class="lang-switch-wrap">`)
	b.WriteString(`<span class="visually-hidden">` + label + `</span>`)
	b.WriteString(`<select class="lang-switch" id="lang-switch" title="` + label + `" aria-label="` + label + `">`)
	for _, opt := range i18n.Locales() {
		sel := ""
		if opt.Code == l.Code() {
			sel = " selected"
		}
		// Each language is named in its own script (English / 简体中文),
		// so the switcher reads the same in every locale.
		b.WriteString(`<option value="` + html.EscapeString(opt.Code) + `"` + sel + `>` + html.EscapeString(opt.Label) + `</option>`)
	}
	b.WriteString(`</select></label>`)
	return b.String()
}
