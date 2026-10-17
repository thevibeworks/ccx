package i18n

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

func TestCatalogCoverage(t *testing.T) {
	enKeys := Keys()
	sort.Strings(enKeys)
	if len(enKeys) == 0 {
		t.Fatal("english catalog is empty")
	}

	for _, code := range []string{ZhCN} {
		cat := Catalog(code)
		if cat == nil {
			t.Fatalf("missing catalog %s", code)
		}
		var missing, empty []string
		for _, k := range enKeys {
			v, ok := cat[k]
			if !ok {
				missing = append(missing, k)
				continue
			}
			if strings.TrimSpace(v) == "" {
				empty = append(empty, k)
			}
		}
		if len(missing) > 0 {
			t.Errorf("%s missing %d keys: %s", code, len(missing), strings.Join(missing, ", "))
		}
		if len(empty) > 0 {
			t.Errorf("%s has empty translations: %s", code, strings.Join(empty, ", "))
		}
		var extra []string
		for k := range cat {
			if _, ok := catalogs[En][k]; !ok {
				extra = append(extra, k)
			}
		}
		if len(extra) > 0 {
			sort.Strings(extra)
			t.Errorf("%s has extra keys not in English: %s", code, strings.Join(extra, ", "))
		}
	}
}

func TestChineseLooksChinese(t *testing.T) {
	// A few chrome strings must actually be Simplified Chinese, not
	// leftover English. Product names and format tokens stay Latin.
	zh := Catalog(ZhCN)
	for _, key := range []string{"nav.projects", "nav.sessions", "settings.heading", "search.heading", "session.outline"} {
		if !containsHan(zh[key]) {
			t.Errorf("%s = %q, want Han characters", key, zh[key])
		}
	}
}

func containsHan(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

func TestTFallbackAndFormat(t *testing.T) {
	en := For(En)
	if got := en.T("nav.projects"); got != "Projects" {
		t.Errorf("en nav.projects = %q", got)
	}
	if got := en.T("projects.stats", 3, 10); got != "3 projects / 10 sessions" {
		t.Errorf("en projects.stats = %q", got)
	}

	zh := For("zh")
	if zh.Code() != ZhCN {
		t.Errorf("For(zh) code = %q, want %s", zh.Code(), ZhCN)
	}
	if got := zh.T("nav.projects"); got != "项目" {
		t.Errorf("zh nav.projects = %q", got)
	}
	if got := zh.T("projects.stats", 3, 10); !strings.Contains(got, "3") || !strings.Contains(got, "项目") {
		t.Errorf("zh projects.stats = %q", got)
	}

	if got := For("nope").T("nav.projects"); got != "Projects" {
		t.Errorf("unknown locale should fall back to English, got %q", got)
	}
	if got := For(ZhCN).T("does.not.exist"); got != "does.not.exist" {
		t.Errorf("missing key should return the key, got %q", got)
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"auto", ""},
		{"en", En},
		{"EN-US", En},
		{"en-CA", En},
		{"en-IN", En},
		{"english", ""},
		{"zh", ZhCN},
		{"zh_CN", ZhCN},
		{"zh-Hans", ZhCN},
		{"zh-Hans-HK", ZhCN},
		{"zh-SG", ZhCN},
		{"zh-TW", ""},
		{"zh-Hant", ""},
		{"fr", ""},
	}
	for _, tt := range tests {
		if got := Normalize(tt.in); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDetectPrecedence(t *testing.T) {
	req := httptest.NewRequest("GET", "/?lang=zh-CN", nil)
	req.Header.Set("Accept-Language", "en")
	req.AddCookie(&http.Cookie{Name: CookieName, Value: En})
	tr, fromQuery := Detect(req, En)
	if tr.Code() != ZhCN || !fromQuery {
		t.Fatalf("query should win: code=%s fromQuery=%v", tr.Code(), fromQuery)
	}

	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "zh"})
	req.Header.Set("Accept-Language", "en")
	tr, fromQuery = Detect(req, En)
	if tr.Code() != ZhCN || fromQuery {
		t.Fatalf("cookie should win over config/header: code=%s fromQuery=%v", tr.Code(), fromQuery)
	}

	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "en")
	tr, _ = Detect(req, "zh-CN")
	if tr.Code() != ZhCN {
		t.Fatalf("config should win over Accept-Language: %s", tr.Code())
	}

	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "fr;q=0.8, zh-CN;q=0.9, en;q=0.5")
	tr, _ = Detect(req, "auto")
	if tr.Code() != ZhCN {
		t.Fatalf("Accept-Language q-value pick = %s, want %s", tr.Code(), ZhCN)
	}

	req = httptest.NewRequest("GET", "/", nil)
	tr, _ = Detect(req, "")
	if tr.Code() != En {
		t.Fatalf("default = %s, want en", tr.Code())
	}
}

func TestAcceptLanguageEnglishRegionsAndUnacceptable(t *testing.T) {
	tests := []struct {
		header, want string
	}{
		{"en-CA,zh;q=0.5", En}, // any English region outranks a lower zh
		{"en-IN,zh-CN;q=0.8", En},
		{"fr,zh;q=0", En}, // q=0 is "not acceptable", never a pick
		{"zh-CN;q=0, en;q=0.1", En},
		{"fr,zh;q=0.5", ZhCN}, // no fr catalog; zh is still acceptable
	}
	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Language", tt.header)
		if tr, _ := Detect(req, ""); tr.Code() != tt.want {
			t.Errorf("Accept-Language %q = %s, want %s", tt.header, tr.Code(), tt.want)
		}
	}
}

// Catalog values are escaped by callers, except keys ending in _markup,
// which carry markup and must only be formatted with escaped arguments.
func TestOnlyHTMLKeysCarryMarkup(t *testing.T) {
	for code, cat := range catalogs {
		for key, value := range cat {
			if strings.Contains(value, "<") != strings.HasSuffix(key, "_markup") {
				t.Errorf("%s %s = %q: markup belongs only in keys ending in _markup", code, key, value)
			}
		}
	}
}
