package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thevibeworks/ccx/internal/i18n"
)

func TestIndexPage_ChineseQuery(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/?lang=zh-CN", nil)
	w := httptest.NewRecorder()
	localeMiddleware(http.HandlerFunc(handleIndex)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`lang="zh-CN"`,
		"项目",
		"会话",
		"设置",
		"简体中文",
		`id="lang-switch"`,
		`window.CCX_LOCALE="zh-CN"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Chinese index missing %q", want)
		}
	}
	if strings.Contains(body, "<h1>Projects</h1>") {
		t.Error("Chinese index still has English Projects heading")
	}
	// Browser translation stays available for languages ccx does not ship.
	for _, unwanted := range []string{`translate="no"`, "notranslate"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("page disables browser translation: found %q", unwanted)
		}
	}
	// Only the keys client JS reads are inlined, not the whole catalog.
	if strings.Contains(body, `"nav.projects"`) || !strings.Contains(body, `"js.copy"`) {
		t.Error("CCX_I18N should carry js.* keys only")
	}

	cookies := w.Result().Cookies()
	var got string
	for _, c := range cookies {
		if c.Name == i18n.CookieName {
			got = c.Value
		}
	}
	if got != i18n.ZhCN {
		t.Errorf("cookie = %q, want %s", got, i18n.ZhCN)
	}
}

func TestIndexPage_AcceptLanguage(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	w := httptest.NewRecorder()
	handleIndex(w, req)

	if !strings.Contains(w.Body.String(), "项目") {
		t.Error("Accept-Language zh-CN should render 项目")
	}
}

func TestIndexPage_CookieBeatsAcceptLanguage(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "zh-CN")
	req.AddCookie(&http.Cookie{Name: i18n.CookieName, Value: i18n.En})
	w := httptest.NewRecorder()
	handleIndex(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "<h1>Projects</h1>") {
		t.Error("cookie en should keep English heading")
	}
	if strings.Contains(body, "<h1>项目</h1>") {
		t.Error("cookie en should not render Chinese heading")
	}
}

func TestSessionsPage_Chinese(t *testing.T) {
	dir := setupMultiProjectDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/sessions?lang=zh", nil)
	w := httptest.NewRecorder()
	handleSessionsPage(w, req)

	body := w.Body.String()
	for _, want := range []string{"会话", "全部提供方", "更多筛选"} {
		if !strings.Contains(body, want) {
			t.Errorf("Chinese sessions page missing %q", want)
		}
	}
}

func TestDefaultLocaleStaysEnglish(t *testing.T) {
	dir := setupTestDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handleIndex(w, req)

	if !strings.Contains(w.Body.String(), "<h1>Projects</h1>") {
		t.Error("default locale should stay English")
	}
}
