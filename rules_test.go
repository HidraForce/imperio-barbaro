package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func adminTestApp(t *testing.T) *app {
	t.Helper()
	db := leadershipTestDB(t, filepath.Join(t.TempDir(), "admin.db"))
	if err := initRules(db); err != nil {
		t.Fatal(err)
	}
	if err := initModeration(db); err != nil {
		t.Fatal(err)
	}
	if err := initRaids(db); err != nil {
		t.Fatal(err)
	}
	if err := initMembers(db); err != nil {
		t.Fatal(err)
	}
	return &app{db: db, staticFS: os.DirFS("web/public"), clanTags: map[string]string{"1": "P0Y8LQ", "2": "QY8L0P"}}
}
func adminTestRouter(a *app) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/rules", a.rules)
	r.Put("/api/rules", a.saveRules)
	r.Get("/api/moderation", a.moderation)
	r.Post("/api/moderation", a.saveModeration)
	r.Put("/api/moderation/{id}", a.saveModeration)
	r.Put("/api/moderation-settings", a.saveModerationSettings)
	r.Post("/api/moderation/sync", a.syncModeration)
	guard := newAccessGuard("leader", "private-admin-test-password")
	return secureHeaders(a.publicContent(publicHome(a.staticFS, guard.middleware(sameOriginAPI(r)), a.renderHome)))
}
func adminTestRequest(handler http.Handler, method, path string, body any, auth bool) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Clanboard-Request", "1")
	if auth {
		req.SetBasicAuth("leader", "private-admin-test-password")
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}
func TestRulesPublishPersistAndEscape(t *testing.T) {
	a := adminTestApp(t)
	handler := adminTestRouter(a)
	doc, err := readRules(context.Background(), a.db)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Clans[0].Topics) != 7 || doc.Clans[0].Topics[0].ID != "regras-principal-respeito" {
		t.Fatal("default rules or anchors lost")
	}
	doc.General.Title = `<script>alert("x")</script>`
	doc.Clans[0].WarSummary = "Nova orientação de guerra"
	doc.Clans[0].DocumentURL = "https://example.com/new-rules.pdf?version=2"
	saved := adminTestRequest(handler, "PUT", "https://dashboard.imperiobarbaro.com.br/api/rules", doc, true)
	if saved.Code != 200 {
		t.Fatal(saved.Code, saved.Body.String())
	}
	stale := adminTestRequest(handler, "PUT", "https://dashboard.imperiobarbaro.com.br/api/rules", doc, true)
	if stale.Code != 409 {
		t.Fatal("stale write accepted", stale.Code)
	}
	if err := initRules(a.db); err != nil {
		t.Fatal(err)
	}
	home := adminTestRequest(handler, "GET", "https://imperiobarbaro.com.br/", nil, false)
	if home.Code != 200 {
		t.Fatal(home.Code, home.Body.String())
	}
	page := home.Body.String()
	for _, want := range []string{"&lt;script&gt;", "Nova orientação de guerra", "https://example.com/new-rules.pdf?version=2", `id="regras-principal-respeito"`} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(page, doc.General.Title) || strings.Contains(page, "/regras-principal.pdf") {
		t.Fatal("unsafe or obsolete content exposed")
	}
	if res := adminTestRequest(handler, "HEAD", "https://imperiobarbaro.com.br/", nil, false); res.Code != 200 || res.Body.Len() != 0 {
		t.Fatal("invalid HEAD")
	}
	public := adminTestRequest(handler, "GET", "https://imperiobarbaro.com.br/public/rules", nil, false)
	if public.Code != 200 || strings.Contains(public.Body.String(), "private-admin") {
		t.Fatal("invalid public rules")
	}
}
func TestRulesRejectInvalidLinksAndPrivateWrites(t *testing.T) {
	a := adminTestApp(t)
	handler := adminTestRouter(a)
	for _, link := range []string{"javascript:alert(1)", "http://example.com/pdf", "https://user:secret@example.com/pdf", "data:text/html,test"} {
		doc, _ := readRules(context.Background(), a.db)
		doc.Clans[0].DocumentURL = link
		if res := adminTestRequest(handler, "PUT", "https://dashboard.imperiobarbaro.com.br/api/rules", doc, true); res.Code != 400 {
			t.Fatalf("accepted %s: %d", link, res.Code)
		}
	}
	for _, path := range []string{"/api/rules", "/api/moderation?clan=1", "/api/moderation-settings?clan=1"} {
		if res := adminTestRequest(handler, "PUT", "https://dashboard.imperiobarbaro.com.br"+path, map[string]any{}, false); res.Code != 401 {
			t.Fatalf("unprotected %s: %d", path, res.Code)
		}
	}
	for _, path := range []string{"/public/rules", "/public/home"} {
		if res := adminTestRequest(handler, "PUT", "https://imperiobarbaro.com.br"+path, map[string]any{}, false); res.Code != 405 {
			t.Fatalf("public write allowed: %d", res.Code)
		}
	}
	for _, path := range []string{"/api/moderation?clan=1", "/regras-principal.pdf", "/regras-secundario.pdf"} {
		if res := adminTestRequest(handler, "GET", "https://imperiobarbaro.com.br"+path, nil, false); res.Code != 404 {
			t.Fatalf("public private path %s: %d", path, res.Code)
		}
	}
	req := httptest.NewRequest("PUT", "https://dashboard.imperiobarbaro.com.br/api/rules", strings.NewReader(`{}`))
	req.SetBasicAuth("leader", "private-admin-test-password")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Clanboard-Request", "1")
	req.Header.Set("Origin", "https://evil.test")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != 403 {
		t.Fatal("cross-origin write accepted")
	}
}
