package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

func leadershipTestDB(t *testing.T, path string) *database {
	t.Helper()
	connection, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	db := &database{DB: connection}
	t.Cleanup(func() { db.Close() })
	if err := initLeadership(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func leadershipTestDocument() leadershipDocument {
	return leadershipDocument{Revision: 1, Items: []leadershipItem{{ID: "main-leader", ClanID: "1", Role: "Líder", Name: "Novo líder", Description: "Organização geral."}}}
}

func leadershipTestSave(a *app, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "https://dashboard.imperiobarbaro.com.br/api/leadership", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	a.saveLeadership(res, req)
	return res
}

func TestLeadershipPersistsEditsAndSeedsOnlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leadership.db")
	db := leadershipTestDB(t, path)
	document, err := readLeadership(context.Background(), db)
	if err != nil || document.Revision != 1 || len(document.Items) != 14 {
		t.Fatalf("initial leadership: %+v, %v", document, err)
	}
	if document.Items[0].Name != "Cyborg" || document.Items[7].Name != "" || document.Items[13].Name != "Aeshton" {
		t.Fatal("initial roster or vacancies changed")
	}
	document.Items[0].Name = "  Nova liderança  "
	document.Items[0].Role = "  Líder  "
	document.Items[0].Description = "  Gestão do clã.  "
	document.Items[1].Name = "  "
	body, _ := json.Marshal(document)
	res := leadershipTestSave(&app{db: db}, body)
	if res.Code != http.StatusOK {
		t.Fatalf("save: %d %s", res.Code, res.Body.String())
	}
	var saved leadershipDocument
	if err := json.Unmarshal(res.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 2 || saved.Items[0].Name != "Nova liderança" || saved.Items[0].Role != "Líder" || saved.Items[0].Description != "Gestão do clã." || saved.Items[1].Name != "" {
		t.Fatalf("normalization/revision: %+v", saved)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := leadershipTestDB(t, path)
	persisted, err := readLeadership(context.Background(), reopened)
	if err != nil || persisted.Revision != 2 || persisted.Items[0].Name != "Nova liderança" || persisted.Items[1].Name != "" {
		t.Fatalf("restart replaced saved leadership: %+v, %v", persisted, err)
	}
}

func TestLeadershipRejectsInvalidWritesWithoutChangingRoster(t *testing.T) {
	db := leadershipTestDB(t, filepath.Join(t.TempDir(), "leadership.db"))
	a := &app{db: db}
	for _, tc := range []struct {
		name   string
		change func(*leadershipDocument)
	}{
		{"missing revision", func(d *leadershipDocument) { d.Revision = 0 }},
		{"missing items", func(d *leadershipDocument) { d.Items = nil }},
		{"too many items", func(d *leadershipDocument) { d.Items = make([]leadershipItem, 41) }},
		{"invalid id", func(d *leadershipDocument) { d.Items[0].ID = "<script>" }},
		{"long id", func(d *leadershipDocument) { d.Items[0].ID = strings.Repeat("a", 65) }},
		{"duplicate id", func(d *leadershipDocument) { d.Items = append(d.Items, d.Items[0]) }},
		{"invalid clan", func(d *leadershipDocument) { d.Items[0].ClanID = "3" }},
		{"blank role", func(d *leadershipDocument) { d.Items[0].Role = " " }},
		{"long role", func(d *leadershipDocument) { d.Items[0].Role = strings.Repeat("á", 101) }},
		{"long name", func(d *leadershipDocument) { d.Items[0].Name = strings.Repeat("á", 101) }},
		{"blank description", func(d *leadershipDocument) { d.Items[0].Description = " " }},
		{"long description", func(d *leadershipDocument) { d.Items[0].Description = strings.Repeat("á", 601) }},
		{"control character", func(d *leadershipDocument) { d.Items[0].Name = "A\x00B" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := leadershipTestDocument()
			tc.change(&document)
			body, _ := json.Marshal(document)
			if res := leadershipTestSave(a, body); res.Code != http.StatusBadRequest {
				t.Fatalf("accepted invalid write: %d %s", res.Code, res.Body.String())
			}
		})
	}
	valid, _ := json.Marshal(leadershipTestDocument())
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"unknown root field", `{"revision":1,"items":[],"phone":"secret"}`, 400},
		{"unknown item field", strings.Replace(string(valid), `"role":`, `"phone":"secret","role":`, 1), 400},
		{"second object", string(valid) + `{}`, 400},
		{"trailing garbage", string(valid) + `nonsense`, 400},
		{"null", `null`, 400},
		{"array", `[]`, 400},
		{"malformed JSON", `{"revision":`, 400},
		{"body limit", `{"revision":1,"items":[],"unknown":"` + strings.Repeat("a", 65536) + `"}`, 413},
		{"trailing body limit", string(valid) + strings.Repeat(" ", 65536), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if res := leadershipTestSave(a, []byte(tc.body)); res.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", res.Code, tc.status, res.Body.String())
			}
		})
	}
	document, err := readLeadership(context.Background(), db)
	if err != nil || document.Revision != 1 || len(document.Items) != 14 {
		t.Fatalf("invalid writes changed roster: %+v, %v", document, err)
	}
}

func TestLeadershipCompareAndSwapPreventsLostUpdates(t *testing.T) {
	db := leadershipTestDB(t, filepath.Join(t.TempDir(), "leadership.db"))
	a := &app{db: db}
	body, _ := json.Marshal(leadershipTestDocument())
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- leadershipTestSave(a, body).Code
		}()
	}
	wg.Wait()
	close(results)
	counts := map[int]int{}
	for status := range results {
		counts[status]++
	}
	if counts[http.StatusOK] != 1 || counts[http.StatusConflict] != 1 {
		t.Fatalf("concurrent writes: %v", counts)
	}
	if res := leadershipTestSave(a, body); res.Code != http.StatusConflict {
		t.Fatalf("stale update: %d", res.Code)
	}
	document, err := readLeadership(context.Background(), db)
	if err != nil || document.Revision != 2 || document.Items[0].Name != "Novo líder" {
		t.Fatalf("conflict changed roster: %+v, %v", document, err)
	}
}

func TestLeadershipPublicEndpointExposesOnlyRosterReads(t *testing.T) {
	db := leadershipTestDB(t, filepath.Join(t.TempDir(), "leadership.db"))
	a := &app{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/leadership", a.leadership)
	mux.HandleFunc("PUT /api/leadership", a.saveLeadership)
	guard := newAccessGuard("leader", "a-long-private-test-password")
	assets := fstest.MapFS{"home.html": {Data: []byte("public home")}}
	handler := secureHeaders(a.publicContent(publicHome(assets, guard.middleware(sameOriginAPI(mux)))))
	for _, tc := range []struct {
		name, method, host, path string
		auth, marker             bool
		status                   int
	}{
		{"main public", "GET", "imperiobarbaro.com.br", "/public/leadership", false, false, 200},
		{"dashboard public", "GET", "dashboard.imperiobarbaro.com.br", "/public/leadership", false, false, 200},
		{"local public", "HEAD", "localhost:2025", "/public/leadership", false, false, 200},
		{"main read only", "PUT", "imperiobarbaro.com.br", "/public/leadership", false, false, 405},
		{"dashboard read only", "PUT", "dashboard.imperiobarbaro.com.br", "/public/leadership", false, false, 401},
		{"exact path", "GET", "dashboard.imperiobarbaro.com.br", "/public/leadership/private", false, false, 401},
		{"no prefix match", "GET", "dashboard.imperiobarbaro.com.br", "/public/leadership-other", false, false, 401},
		{"private contacts", "GET", "dashboard.imperiobarbaro.com.br", "/api/contacts", false, false, 401},
		{"admin read protected", "GET", "dashboard.imperiobarbaro.com.br", "/api/leadership", false, false, 401},
		{"admin write protected", "PUT", "dashboard.imperiobarbaro.com.br", "/api/leadership", false, true, 401},
		{"authenticated read", "GET", "dashboard.imperiobarbaro.com.br", "/api/leadership", true, false, 200},
		{"write marker required", "PUT", "dashboard.imperiobarbaro.com.br", "/api/leadership", true, false, 403},
		{"authenticated write", "PUT", "dashboard.imperiobarbaro.com.br", "/api/leadership", true, true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(leadershipTestDocument())
			req := httptest.NewRequest(tc.method, "https://"+tc.host+tc.path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tc.auth {
				req.SetBasicAuth("leader", "a-long-private-test-password")
			}
			if tc.marker {
				req.Header.Set("X-Clanboard-Request", "1")
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", res.Code, tc.status, res.Body.String())
			}
			if tc.method == "HEAD" && res.Body.Len() != 0 {
				t.Fatal("HEAD returned a body")
			}
			if res.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("leadership can be cached after edits")
			}
			if tc.status == 200 && tc.method == "GET" {
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(res.Body.Bytes(), &fields); err != nil || len(fields) != 2 || fields["revision"] == nil || fields["items"] == nil {
					t.Fatalf("unexpected public fields: %s", res.Body.String())
				}
				var items []map[string]json.RawMessage
				if err := json.Unmarshal(fields["items"], &items); err != nil {
					t.Fatal(err)
				}
				for _, item := range items {
					if len(item) != 5 || item["id"] == nil || item["clanId"] == nil || item["role"] == nil || item["name"] == nil || item["description"] == nil {
						t.Fatalf("unexpected public item fields: %v", item)
					}
				}
			}
		})
	}
}

func TestLeadershipJSONEscapesMarkupAndUsesCharacterLimits(t *testing.T) {
	db := leadershipTestDB(t, filepath.Join(t.TempDir(), "leadership.db"))
	document := leadershipTestDocument()
	document.Items[0].Name = `<script>alert("test")</script>`
	document.Items[0].Role = strings.Repeat("á", 100)
	document.Items[0].Description = strings.Repeat("ç", 600)
	body, _ := json.Marshal(document)
	res := leadershipTestSave(&app{db: db}, body)
	if res.Code != http.StatusOK || strings.Contains(res.Body.String(), "<script>") || !strings.Contains(res.Body.String(), `\u003cscript\u003e`) {
		t.Fatalf("markup not escaped or unicode limits rejected: %d %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &document); err != nil || document.Items[0].Name != `<script>alert("test")</script>` {
		t.Fatal("plain text did not round trip")
	}
}
