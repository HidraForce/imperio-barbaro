package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

func validCredentials(username, password string) bool {
	if strings.TrimSpace(username) == "" || len(username) > 128 || len(password) < 16 || len(password) > 1024 {
		return false
	}
	for _, prefix := range []string{"replace-this", "change-this", "defina-uma-senha"} {
		if strings.HasPrefix(strings.ToLower(password), prefix) {
			return false
		}
	}
	return true
}

func securityError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' https://api-assets.clashofclans.com data:; connect-src 'self'; font-src 'self'; worker-src 'self'; manifest-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("Cache-Control", "no-store")
		// O proxy HTTPS deve configurar HSTS; cabeçalhos encaminhados não são confiados.
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

// O cabeçalho personalizado impede formulários de outros sites de gravar contatos.
// Não habilitamos CORS: a pré-verificação de outro domínio nunca recebe permissão.
func sameOriginAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" || site == "same-site" {
			securityError(w, 403, "Esta consulta deve ser feita pelo próprio painel.")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, r.Host) || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
				securityError(w, 403, "Origem da consulta não autorizada.")
				return
			}
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get("X-Clanboard-Request") != "1" {
				securityError(w, 403, "Proteção de gravação ausente. Atualize o painel e tente novamente.")
				return
			}
			if r.Method == http.MethodPost {
				mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || mediaType != "application/json" {
					securityError(w, 415, "Envie os dados no formato JSON.")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

type failedAccess struct {
	count   int
	expires time.Time
}
type accessGuard struct {
	mu                 sync.Mutex
	failures           map[string]failedAccess
	username, password [32]byte
	now                func() time.Time
}

func newAccessGuard(username, password string) *accessGuard {
	return &accessGuard{failures: make(map[string]failedAccess), username: sha256.Sum256([]byte(username)), password: sha256.Sum256([]byte(password)), now: time.Now}
}

func (g *accessGuard) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		now := g.now()
		g.mu.Lock()
		for key, entry := range g.failures {
			if !now.Before(entry.expires) {
				delete(g.failures, key)
			}
		}
		entry := g.failures[ip]
		if entry.count >= 10 {
			g.mu.Unlock()
			w.Header().Set("Retry-After", strconv.Itoa(int(entry.expires.Sub(now).Seconds())+1))
			securityError(w, 429, "Muitas tentativas de acesso. Aguarde alguns minutos.")
			return
		}
		username, password, ok := r.BasicAuth()
		userHash, passwordHash := sha256.Sum256([]byte(username)), sha256.Sum256([]byte(password))
		valid := subtle.ConstantTimeCompare(userHash[:], g.username[:]) & subtle.ConstantTimeCompare(passwordHash[:], g.password[:])
		if !ok || valid != 1 {
			// A primeira navegação sem credenciais apenas abre o diálogo de acesso.
			if r.Header.Get("Authorization") != "" {
				if entry.count == 0 {
					if len(g.failures) >= 2048 {
						g.mu.Unlock()
						w.Header().Set("Retry-After", "60")
						securityError(w, 429, "Acesso temporariamente limitado. Tente novamente em um minuto.")
						return
					}
					entry.expires = now.Add(5 * time.Minute)
				}
				entry.count++
				g.failures[ip] = entry
			}
			g.mu.Unlock()
			w.Header().Set("WWW-Authenticate", `Basic realm="Império Bárbaro", charset="UTF-8"`)
			securityError(w, 401, "Informe o usuário e a senha do painel.")
			return
		}
		g.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

func secureServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
}
