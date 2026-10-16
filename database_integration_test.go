package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func integrationDatabase(t *testing.T, postgres bool) *database {
	t.Helper()
	var raw *sql.DB
	var err error
	if postgres {
		dsn := os.Getenv("TEST_DATABASE_URL")
		if dsn == "" {
			t.Skip("Configure TEST_DATABASE_URL com um PostgreSQL descartável para os testes de integração")
		}
		control, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		schema := fmt.Sprintf("crmtest_%d", time.Now().UnixNano())
		if _, err = control.Exec("CREATE SCHEMA " + schema); err != nil {
			t.Fatal(err)
		}
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		raw, err = sql.Open("pgx", parsed.String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { raw.Close(); control.Exec("DROP SCHEMA " + schema + " CASCADE"); control.Close() })
	} else {
		raw, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		raw.SetMaxOpenConns(1)
		t.Cleanup(func() { raw.Close() })
	}
	db := &database{raw, postgres}
	if err = initContacts(db); err != nil {
		t.Fatal(err)
	}
	if err = initRaids(db); err != nil {
		t.Fatal(err)
	}
	return db
}
func TestContactsPersistenceAcrossDatabases(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprint("postgres=", postgres), func(t *testing.T) {
			db := integrationDatabase(t, postgres)
			ctx := context.Background()
			first := contact{RealName: "Bruno", Phone: "5511999999999", Notes: "Líder", Accounts: []contactAccount{{PlayerTag: "#PYQ", PlayerName: "Principal", ClanID: "1"}, {PlayerTag: "#YQL", PlayerName: "Segunda", ClanID: "2"}}}
			second := contact{RealName: "Outro cadastro", Phone: "5511988888888", Notes: "Apoio", Accounts: []contactAccount{{PlayerTag: "#QPC", PlayerName: "Terceira", ClanID: "1"}}}
			if err := writeContact(ctx, db, &first); err != nil {
				t.Fatal(err)
			}
			if err := writeContact(ctx, db, &second); err != nil {
				t.Fatal(err)
			}
			edit := first
			edit.Accounts = []contactAccount{{PlayerTag: "#QPC", PlayerName: "Duplicada", ClanID: "1"}}
			if err := writeContact(ctx, db, &edit); err != errAccountInUse {
				t.Fatalf("colisão: %v", err)
			}
			list, err := listContacts(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			var kept contact
			for _, c := range list {
				if c.ID == first.ID {
					kept = c
				}
			}
			if len(kept.Accounts) != 2 {
				t.Fatal("rollback não preservou as contas anteriores")
			}
			if err := mergeContacts(ctx, db, first.ID, second.ID); err != nil {
				t.Fatal(err)
			}
			list, err = listContacts(ctx, db)
			if err != nil || len(list) != 1 || len(list[0].Accounts) != 3 || !strings.Contains(list[0].Notes, second.Phone) {
				t.Fatalf("reunião: %v %v", list, err)
			}
			if err = eraseContact(ctx, db, first.ID); err != nil {
				t.Fatal(err)
			}
			list, err = listContacts(ctx, db)
			if err != nil || len(list) != 0 {
				t.Fatal("exclusão incompleta")
			}
		})
	}
}
func TestPostgresContactSessionsAndAttemptsAreShared(t *testing.T) {
	db := integrationDatabase(t, true)
	secret := "uma-senha-adicional-privada"
	a, b := newContactVault(secret), newContactVault(secret)
	a.store = db
	b.store = db
	unlock := httptest.NewRecorder()
	a.unlock(unlock, httptest.NewRequest("POST", "/api/contacts/unlock", strings.NewReader(`{"password":"`+secret+`"}`)))
	if unlock.Code != 200 {
		t.Fatalf("unlock: %d %s", unlock.Code, unlock.Body)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(unlock.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/contacts", nil)
	request.Header.Set("X-Contacts-Session", result.Token)
	reached := false
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })
	b.protect(handler).ServeHTTP(httptest.NewRecorder(), request)
	if !reached {
		t.Fatal("outra instância não reconheceu a sessão")
	}
	b.lock(httptest.NewRecorder(), request)
	reached = false
	response := httptest.NewRecorder()
	a.protect(handler).ServeHTTP(response, request)
	if reached || response.Code != 423 {
		t.Fatal("revogação não foi compartilhada")
	}
	secondUnlock := httptest.NewRecorder()
	a.unlock(secondUnlock, httptest.NewRequest("POST", "/api/contacts/unlock", strings.NewReader(`{"password":"`+secret+`"}`)))
	if secondUnlock.Code != 200 {
		t.Fatal("não foi possível abrir a segunda sessão")
	}
	if err := json.Unmarshal(secondUnlock.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Contacts-Session", result.Token)
	b.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	response = httptest.NewRecorder()
	b.protect(handler).ServeHTTP(response, request)
	if response.Code != 423 {
		t.Fatal("sessão remota não expirou")
	}
	g1, g2 := newAccessGuard("lider", secret), newAccessGuard("lider", secret)
	g1.store = db
	g2.store = db
	for i := 0; i < 11; i++ {
		req := httptest.NewRequest("GET", "/api/health", nil)
		req.SetBasicAuth("lider", "incorreta")
		res := httptest.NewRecorder()
		guard := g1
		if i%2 == 1 {
			guard = g2
		}
		guard.middleware(handler).ServeHTTP(res, req)
		expected := 401
		if i == 10 {
			expected = 429
		}
		if res.Code != expected {
			t.Fatalf("tentativa %d: %d", i, res.Code)
		}
	}
}
func TestPostgresCacheCoalescesAcrossInstancesAndSharesCooldown(t *testing.T) {
	db := integrationDatabase(t, true)
	var calls atomic.Int32
	transport := clashTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		return clashResponse(200, `{"name":"Clã"}`), nil
	})
	a, b := testClash(transport), testClash(transport)
	a.db = db
	b.db = db
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			instance := a
			if i%2 == 1 {
				instance = b
			}
			if _, status, err := instance.fetchClash(context.Background(), "/clans/%23PYQ"); err != nil || status != 200 {
				t.Errorf("cache: %d %v", status, err)
			}
		}(i)
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("consultas simultâneas: %d", calls.Load())
	}
	if !a.clashFetchedAt("/clans/%23PYQ").Equal(b.clashFetchedAt("/clans/%23PYQ")) {
		t.Fatal("data de consulta foi alterada ao ler cache")
	}
	if _, err := db.Exec(`UPDATE crm_cache SET expires=0 WHERE key=?`, a.cacheNamespace()+"/clans/%23PYQ"); err != nil {
		t.Fatal(err)
	}
	if _, status, err := b.fetchClash(context.Background(), "/clans/%23PYQ"); err != nil || status != 200 {
		t.Fatal("falha ao renovar o cache expirado")
	}
	if calls.Load() != 2 {
		t.Fatal("cache expirado foi reutilizado")
	}
	a.clash.Transport = clashTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		response := clashResponse(429, `{}`)
		response.Header.Set("Retry-After", "120")
		return response, nil
	})
	if _, status, _ := a.fetchClash(context.Background(), "/clans/%23YQL"); status != 429 {
		t.Fatal("limite da API não registrado")
	}
	if _, status, _ := b.fetchClash(context.Background(), "/clans/%23QPC"); status != 429 {
		t.Fatal("pausa não reconhecida na outra instância")
	}
	if calls.Load() != 3 {
		t.Fatal("API foi consultada durante a pausa compartilhada")
	}
}

func TestRulesAndModerationAcrossDatabases(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprint("postgres=", postgres), func(t *testing.T) {
			db := integrationDatabase(t, postgres)
			if err := initRules(db); err != nil {
				t.Fatal(err)
			}
			if err := initModeration(db); err != nil {
				t.Fatal(err)
			}
			a := &app{db: db, clanTags: map[string]string{"1": "P0Y8LQ", "2": "QY8L0P"}}
			handler := adminTestRouter(a)
			rules, err := readRules(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			rules.General.Title = "Regras atualizadas"
			if result := adminTestRequest(handler, "PUT", "https://dashboard.imperiobarbaro.com.br/api/rules", rules, true); result.Code != 200 {
				t.Fatal(result.Code, result.Body.String())
			}
			if err = initRules(db); err != nil {
				t.Fatal(err)
			}
			saved, err := readRules(context.Background(), db)
			if err != nil || saved.Revision != 2 || saved.General.Title != "Regras atualizadas" {
				t.Fatal("rules edits lost", saved, err)
			}
			record := moderationRecord{PlayerTag: "#P0Y8LQ", PlayerName: "Jogador", Kind: "warning", Status: "active", Reason: "Zero ataques.", Source: "war", EventKey: "war:event:P0Y8LQ"}
			added, err := insertModeration(context.Background(), db, "P0Y8LQ", &record)
			if err != nil || !added {
				t.Fatal("insert failed", added, err)
			}
			added, err = insertModeration(context.Background(), db, "P0Y8LQ", &record)
			if err != nil || added {
				t.Fatal("duplicate inserted", added, err)
			}
			if result := adminTestRequest(handler, "PUT", "https://dashboard.imperiobarbaro.com.br/api/moderation-settings?clan=1", map[string]any{"revision": 1, "automatic": true}, true); result.Code != 200 {
				t.Fatal(result.Code, result.Body.String())
			}
			if result := adminTestRequest(handler, "PUT", "https://dashboard.imperiobarbaro.com.br/api/moderation-settings?clan=1", map[string]any{"revision": 2, "automatic": false}, true); result.Code != 200 {
				t.Fatal(result.Code, result.Body.String())
			}
			settings, err := readModerationSettings(context.Background(), db, "P0Y8LQ")
			if err != nil || settings.Revision != 3 || settings.Automatic {
				t.Fatal("settings update failed", settings, err)
			}
		})
	}
}
