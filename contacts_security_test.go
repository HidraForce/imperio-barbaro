package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const vaultTestPassword = "senha-exclusiva-de-contatos-teste"

func unlockVault(t *testing.T, v *contactVault, password, ip string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"password": password})
	r := httptest.NewRequest("POST", "http://painel.test/api/contacts/unlock", strings.NewReader(string(body)))
	r.RemoteAddr = ip + ":1234"
	w := httptest.NewRecorder()
	v.unlock(w, r)
	var result struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	return w, result.Token
}

func protectedVault(v *contactVault, method, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://painel.test/api/contacts", nil)
	r.Header.Set("X-Contacts-Session", token)
	w := httptest.NewRecorder()
	v.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
	return w
}

func TestContactsRequireSessionForReadingWritingAndDeleting(t *testing.T) {
	v := newContactVault(vaultTestPassword)
	for _, method := range []string{"GET", "POST", "DELETE"} {
		if w := protectedVault(v, method, ""); w.Code != 423 {
			t.Errorf("%s foi liberado sem sessão: %d", method, w.Code)
		}
	}
	w, token := unlockVault(t, v, vaultTestPassword, "192.0.2.1")
	if w.Code != 200 || len(token) != 43 {
		t.Fatalf("desbloqueio falhou: %d", w.Code)
	}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		if w := protectedVault(v, method, token); w.Code != 204 {
			t.Errorf("%s não foi liberado: %d", method, w.Code)
		}
	}
	if protectedVault(v, "GET", strings.Repeat("x", 43)).Code != 423 {
		t.Fatal("token não emitido foi aceito")
	}
}

func TestContactsWithoutConfigurationFailClosed(t *testing.T) {
	v := newContactVault("")
	if protectedVault(v, "GET", "").Code != 503 {
		t.Fatal("contatos sem configuração foram liberados")
	}
	w, _ := unlockVault(t, v, vaultTestPassword, "192.0.2.1")
	if w.Code != 503 {
		t.Fatal("senha ausente permitiu desbloqueio")
	}
}

func TestContactSessionsExpireWithoutSlidingRenewal(t *testing.T) {
	v := newContactVault(vaultTestPassword)
	now := time.Now()
	v.now = func() time.Time { return now }
	_, token := unlockVault(t, v, vaultTestPassword, "192.0.2.1")
	now = now.Add(9 * time.Minute)
	if protectedVault(v, "GET", token).Code != 204 {
		t.Fatal("sessão expirou antes do prazo")
	}
	now = now.Add(time.Minute)
	if protectedVault(v, "GET", token).Code != 423 {
		t.Fatal("consulta renovou o prazo absoluto")
	}
	if len(v.sessions) != 0 {
		t.Fatal("sessão expirada não foi removida")
	}
}

func TestLockRevokesOnlyTheCurrentContactSession(t *testing.T) {
	v := newContactVault(vaultTestPassword)
	_, first := unlockVault(t, v, vaultTestPassword, "192.0.2.1")
	_, second := unlockVault(t, v, vaultTestPassword, "192.0.2.2")
	if first == second {
		t.Fatal("tokens reutilizados")
	}
	r := httptest.NewRequest("DELETE", "http://painel.test/api/contacts/session", nil)
	r.Header.Set("X-Contacts-Session", first)
	w := httptest.NewRecorder()
	v.lock(w, r)
	if w.Code != 204 || protectedVault(v, "GET", first).Code != 423 || protectedVault(v, "GET", second).Code != 204 {
		t.Fatal("revogação incorreta")
	}
	if protectedVault(newContactVault(vaultTestPassword), "GET", second).Code != 423 {
		t.Fatal("reinicialização preservou uma sessão antiga")
	}
}

func TestContactPasswordAttemptsAreLimitedAndExpire(t *testing.T) {
	v := newContactVault(vaultTestPassword)
	now := time.Now()
	v.now = func() time.Time { return now }
	for i := 0; i < 5; i++ {
		w, _ := unlockVault(t, v, "incorreta", "192.0.2.1")
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	w, _ := unlockVault(t, v, vaultTestPassword, "192.0.2.1")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("tentativas não limitadas")
	}
	w, _ = unlockVault(t, v, vaultTestPassword, "192.0.2.2")
	if w.Code != 200 {
		t.Fatal("outro endereço foi bloqueado")
	}
	now = now.Add(5 * time.Minute)
	w, _ = unlockVault(t, v, vaultTestPassword, "192.0.2.1")
	if w.Code != 200 {
		t.Fatal("limite não expirou")
	}
}

func TestMalformedUnlockPayloadDoesNotCreateSession(t *testing.T) {
	v := newContactVault(vaultTestPassword)
	for _, body := range []string{`{}`, `{"password":"x","extra":true}`, `{"password":"x"}{}`, `{"password":"` + strings.Repeat("x", 3000) + `"}`} {
		r := httptest.NewRequest("POST", "http://painel.test/api/contacts/unlock", strings.NewReader(body))
		w := httptest.NewRecorder()
		v.unlock(w, r)
		if w.Code == 200 || len(v.sessions) != 0 {
			t.Fatal("payload inválido permitiu acesso")
		}
	}
}

func TestContactSessionsAreSafeUnderConcurrentRequests(t *testing.T) {
	v := newContactVault(vaultTestPassword)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, token := unlockVault(t, v, vaultTestPassword, "192.0.2.1")
			if w.Code != 200 || protectedVault(v, "GET", token).Code != 204 {
				t.Error("consulta simultânea falhou")
			}
		}()
	}
	wg.Wait()
	if len(v.sessions) != 20 {
		t.Fatal("sessões simultâneas foram perdidas")
	}
}

func TestContactUnlockStillRequiresPanelAuthenticationAndCSRFProtection(t *testing.T) {
	v := newContactVault(vaultTestPassword)
	h := secureHeaders(newAccessGuard("lider", "senha-principal-privada-teste").middleware(sameOriginAPI(http.HandlerFunc(v.unlock))))
	for _, tc := range []struct {
		auth, marker bool
		want         int
	}{{false, false, 401}, {true, false, 403}, {true, true, 200}} {
		body, _ := json.Marshal(map[string]string{"password": vaultTestPassword})
		r := httptest.NewRequest("POST", "http://painel.test/api/contacts/unlock", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		if tc.auth {
			r.SetBasicAuth("lider", "senha-principal-privada-teste")
		}
		if tc.marker {
			r.Header.Set("X-Clanboard-Request", "1")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("status %d, esperado %d", w.Code, tc.want)
		}
	}
}
