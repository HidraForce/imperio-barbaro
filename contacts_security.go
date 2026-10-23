package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const contactSessionTTL = 10 * time.Minute

type contactVault struct {
	store                  *database
	scope, header, envName string
	mu                     sync.Mutex
	configured             bool
	password               [32]byte
	sessions               map[[32]byte]time.Time
	failures               map[string]failedAccess
	now                    func() time.Time
}

func newContactVault(password string) *contactVault {
	return newScopedVault(password, "contacts", "X-Contacts-Session", "CONTACTS_PASSWORD")
}

// Each capability has its own header, session namespace and attempt budget.
func newScopedVault(password, scope, header, envName string) *contactVault {
	return &contactVault{scope: scope, header: header, envName: envName, configured: validCredentials("protegido", password), password: sha256.Sum256([]byte(password)), sessions: make(map[[32]byte]time.Time), failures: make(map[string]failedAccess), now: time.Now}
}

func (v *contactVault) prune(now time.Time) {
	for key, expires := range v.sessions {
		if !now.Before(expires) {
			delete(v.sessions, key)
		}
	}
	for key, entry := range v.failures {
		if !now.Before(entry.expires) {
			delete(v.failures, key)
		}
	}
}

func (v *contactVault) unlock(w http.ResponseWriter, r *http.Request) {
	if !v.configured {
		securityError(w, 503, "Acesso bloqueado. Configure "+v.envName+" no servidor.")
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		securityError(w, 400, "Dados de desbloqueio inválidos.")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		securityError(w, 400, "Dados de desbloqueio inválidos.")
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if v.store != nil {
		v.unlockShared(w, r, input.Password, ip)
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.now()
	v.prune(now)
	entry := v.failures[ip]
	if entry.count >= 5 {
		w.Header().Set("Retry-After", strconv.Itoa(int(entry.expires.Sub(now).Seconds())+1))
		securityError(w, 429, "Muitas tentativas de desbloqueio. Aguarde cinco minutos.")
		return
	}
	guess := sha256.Sum256([]byte(input.Password))
	if subtle.ConstantTimeCompare(guess[:], v.password[:]) != 1 {
		if entry.count == 0 {
			if len(v.failures) >= 2048 {
				w.Header().Set("Retry-After", "60")
				securityError(w, 429, "Desbloqueio temporariamente limitado. Tente novamente em um minuto.")
				return
			}
			entry.expires = now.Add(5 * time.Minute)
		}
		entry.count++
		v.failures[ip] = entry
		securityError(w, 403, "Senha incorreta para este acesso.")
		return
	}
	if len(v.sessions) >= 256 {
		w.Header().Set("Retry-After", "60")
		securityError(w, 429, "Há muitas sessões abertas. Tente novamente em um minuto.")
		return
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		securityError(w, 500, "Não foi possível desbloquear este acesso.")
		return
	}
	token := base64.RawURLEncoding.EncodeToString(random[:])
	expires := now.Add(contactSessionTTL)
	v.sessions[sha256.Sum256([]byte(token))] = expires
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"token": token, "expiresAt": expires.UTC().Format(time.RFC3339)})
}

func (v *contactVault) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !v.configured {
			securityError(w, 503, "Acesso bloqueado. Configure "+v.envName+" no servidor.")
			return
		}
		if v.store != nil {
			v.protectShared(w, r, next)
			return
		}
		token := r.Header.Get(v.header)
		key := sha256.Sum256([]byte(token))
		v.mu.Lock()
		now := v.now()
		v.prune(now)
		expires, ok := v.sessions[key]
		v.mu.Unlock()
		if len(token) != 43 || !ok || !now.Before(expires) {
			securityError(w, 423, "Acesso bloqueado. Informe a senha exclusiva para continuar.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (v *contactVault) lock(w http.ResponseWriter, r *http.Request) {
	if v.store != nil {
		v.lockShared(w, r)
		return
	}
	v.mu.Lock()
	delete(v.sessions, sha256.Sum256([]byte(r.Header.Get(v.header))))
	v.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
