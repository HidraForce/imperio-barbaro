package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestEditorsUseSeparatePasswordsAndKeepPublishedReadsPublic(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprint("postgres=", postgres), func(t *testing.T) {
			a := &app{db: integrationDatabase(t, postgres)}
			if err := initLeadership(a.db); err != nil {
				t.Fatal(err)
			}
			if err := initRules(a.db); err != nil {
				t.Fatal(err)
			}
			const leadershipPassword = "private-leadership-password"
			const rulesPassword = "private-rules-password"
			router := chi.NewRouter()
			router.Use(newAccessGuard("leader", "private-admin-test-password").middleware, sameOriginAPI)
			if err := a.editorAccessRoutes(router, leadershipPassword, rulesPassword); err != nil {
				t.Fatal(err)
			}
			handler := a.publicContent(router)
			request := func(method, path, token string, body []byte) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, "https://dashboard.imperiobarbaro.com.br"+path, bytes.NewReader(body))
				req.SetBasicAuth("leader", "private-admin-test-password")
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Clanboard-Request", "1")
				req.Header.Set(editorSessionHeader, token)
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				return res
			}
			tokens := make(map[string]string)
			for _, editor := range []struct{ name, password, otherPassword string }{
				{"leadership", leadershipPassword, rulesPassword},
				{"rules", rulesPassword, leadershipPassword},
			} {
				path := "/api/" + editor.name
				for _, method := range []string{"GET", "PUT"} {
					if res := request(method, path, "", nil); res.Code != 423 {
						t.Fatalf("%s %s without session: %d", method, path, res.Code)
					}
				}
				wrong := adminTestRequest(handler, "POST", path+"/unlock", map[string]string{"password": editor.otherPassword}, true)
				if wrong.Code != 403 {
					t.Fatalf("accepted a different editor's password: %d", wrong.Code)
				}
				unlocked := adminTestRequest(handler, "POST", path+"/unlock", map[string]string{"password": editor.password}, true)
				var session struct{ Token string }
				if err := json.Unmarshal(unlocked.Body.Bytes(), &session); err != nil || unlocked.Code != 200 || session.Token == "" {
					t.Fatalf("unlock %s: %d %s", editor.name, unlocked.Code, unlocked.Body)
				}
				tokens[editor.name] = session.Token
			}
			for _, editor := range []struct{ name, other string }{{"leadership", "rules"}, {"rules", "leadership"}} {
				path := "/api/" + editor.name
				token := tokens[editor.name]
				for _, method := range []string{"GET", "PUT"} {
					if res := request(method, path, tokens[editor.other], nil); res.Code != 423 {
						t.Fatalf("%s %s used another editor's session: %d", method, path, res.Code)
					}
				}
				loaded := request("GET", path, token, nil)
				if loaded.Code != 200 {
					t.Fatalf("load %s: %d %s", editor.name, loaded.Code, loaded.Body)
				}
				if saved := request("PUT", path, token, loaded.Body.Bytes()); saved.Code != 200 {
					t.Fatalf("save %s: %d %s", editor.name, saved.Code, saved.Body)
				}
				if res := request("DELETE", path+"/session", token, nil); res.Code != 204 {
					t.Fatalf("lock %s: %d", editor.name, res.Code)
				}
				if res := request("PUT", path, token, loaded.Body.Bytes()); res.Code != 423 {
					t.Fatalf("revoked %s session: %d", editor.name, res.Code)
				}
				public := adminTestRequest(handler, "GET", "/public/"+editor.name, nil, false)
				if public.Code != 200 || !strings.Contains(public.Body.String(), `"revision":2`) {
					t.Fatalf("public %s: %d %s", editor.name, public.Code, public.Body)
				}
				if res := adminTestRequest(handler, "PUT", "/public/"+editor.name, json.RawMessage(loaded.Body.Bytes()), false); res.Code != 401 && res.Code != 405 {
					t.Fatalf("public %s allows editing: %d", editor.name, res.Code)
				}
			}
		})
	}
}

func TestEditorWithoutPasswordIsLocked(t *testing.T) {
	a := &app{db: integrationDatabase(t, false)}
	router := chi.NewRouter()
	if err := a.editorAccessRoutes(router, "", ""); err != nil {
		t.Fatal(err)
	}
	for name, envName := range map[string]string{"leadership": "LEADERSHIP", "rules": "RULES_PASS"} {
		path := "/api/" + name
		unlock := adminTestRequest(router, "POST", path+"/unlock", map[string]string{"password": "anything"}, true)
		if unlock.Code != 503 || !strings.Contains(unlock.Body.String(), envName) {
			t.Fatalf("missing %s password: %d %s", name, unlock.Code, unlock.Body)
		}
		if save := adminTestRequest(router, "PUT", path, map[string]bool{}, true); save.Code != 503 {
			t.Fatalf("unconfigured %s editor accepted a write: %d", name, save.Code)
		}
	}
}
