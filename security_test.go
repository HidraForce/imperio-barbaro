package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSecurityHeaders(t *testing.T) {
	r := httptest.NewRequest("GET", "https://painel.test/", nil)
	w := httptest.NewRecorder()
	secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(w, r)
	for key, want := range map[string]string{"X-Frame-Options": "DENY", "X-Content-Type-Options": "nosniff", "Cache-Control": "no-store", "Referrer-Policy": "no-referrer", "Strict-Transport-Security": "max-age=31536000"} {
		if w.Header().Get(key) != want {
			t.Errorf("%s: %s", key, w.Header().Get(key))
		}
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'self'") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("CSP incompleta")
	}
}

func TestCrossSiteRequestsAndUnprotectedWritesAreBlocked(t *testing.T) {
	for _, tc := range []struct {
		name, method, origin, site, marker, content string
		want                                        int
	}{
		{"formulário externo", "POST", "https://outro.test", "cross-site", "", "text/plain", 403},
		{"consulta externa", "GET", "", "cross-site", "", "", 403},
		{"subdomínio", "DELETE", "", "same-site", "1", "", 403},
		{"origem inválida", "DELETE", "null", "", "1", "", 403},
		{"domínio externo", "DELETE", "https://outro.test", "", "1", "", 403},
		{"sem cabeçalho", "DELETE", "https://painel.test", "same-origin", "", "", 403},
		{"tipo simples", "POST", "https://painel.test", "same-origin", "1", "text/plain", 415},
		{"gravação válida", "POST", "https://painel.test", "same-origin", "1", "application/json; charset=utf-8", 204},
		{"exclusão válida", "DELETE", "https://painel.test", "same-origin", "1", "", 204},
		{"consulta local", "GET", "", "same-origin", "", "", 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "https://painel.test/api/contacts", nil)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			r.Header.Set("X-Clanboard-Request", tc.marker)
			r.Header.Set("Content-Type", tc.content)
			w := httptest.NewRecorder()
			sameOriginAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Errorf("status %d, esperado %d", w.Code, tc.want)
			}
		})
	}
}

func TestAuthenticationThrottlingExpiresAndIgnoresForwardedIP(t *testing.T) {
	g := newAccessGuard("lider", "uma-senha-privada-de-teste")
	now := time.Now()
	g.now = func() time.Time { return now }
	h := g.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	request := func(ip, password string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://painel.test/api/clan", nil)
		r.RemoteAddr = ip + ":1234"
		r.Header.Set("X-Forwarded-For", password)
		r.SetBasicAuth("lider", password)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for i := 0; i < 10; i++ {
		if w := request("192.0.2.1", "errada"); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	if w := request("192.0.2.1", "outra"); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("tentativas não foram limitadas")
	}
	if w := request("192.0.2.2", "uma-senha-privada-de-teste"); w.Code != 204 {
		t.Fatal("outro endereço foi bloqueado")
	}
	now = now.Add(6 * time.Minute)
	if w := request("192.0.2.1", "uma-senha-privada-de-teste"); w.Code != 204 {
		t.Fatal("bloqueio não expirou")
	}
}

func TestKnownPlaceholderPasswordsAreRejected(t *testing.T) {
	for _, password := range []string{"curta", "change-this-to-a-long-random-password", "replace-this-password", "defina-uma-senha-privada"} {
		if validCredentials("lider", password) {
			t.Errorf("senha de exemplo aceita: %s", password)
		}
	}
	if validCredentials(" ", "uma-senha-privada-de-teste") || !validCredentials("lider", "uma-senha-privada-de-teste") {
		t.Fatal("validação incorreta")
	}
}

func TestServerHasResourceLimits(t *testing.T) {
	s := secureServer(":2025", http.NotFoundHandler())
	if s.ReadHeaderTimeout <= 0 || s.ReadTimeout <= 0 || s.WriteTimeout <= 0 || s.IdleTimeout <= 0 || s.MaxHeaderBytes > 16<<10 {
		t.Fatal("limites ausentes")
	}
}

func TestContactValidation(t *testing.T) {
	c := contact{PlayerTag: " p0y8lq ", PlayerName: " Líder ", Phone: "+55 11 99999-0000"}
	if err := validateContact(&c); err != nil || c.PlayerTag != "#P0Y8LQ" || c.PlayerName != "Líder" {
		t.Fatalf("contato válido rejeitado: %v", err)
	}
	for _, bad := range []contact{{PlayerTag: "#ABC123", PlayerName: "Nome"}, {PlayerTag: "#P0Y8LQ"}, {PlayerTag: "#P0Y8LQ", PlayerName: strings.Repeat("á", 101)}, {PlayerTag: "#P0Y8LQ", PlayerName: "Nome\x00"}, {PlayerTag: "#P0Y8LQ", PlayerName: "Nome", Phone: "javascript:alert(1)"}, {PlayerTag: "#P0Y8LQ", PlayerName: "Nome", Phone: "-------"}} {
		if validateContact(&bad) == nil {
			t.Errorf("contato inválido aceito: %+v", bad)
		}
	}
}
