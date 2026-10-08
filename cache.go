package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type clashCacheEntry struct {
	body                 []byte
	status               int
	err                  error
	fetchedAt, expiresAt time.Time
}

type clashFlight struct {
	done  chan struct{}
	entry clashCacheEntry
}

func clashTTL(endpoint string) time.Duration {
	switch {
	case strings.Contains(endpoint, "capitalraidseasons"):
		return 15 * time.Minute
	case strings.Contains(endpoint, "currentwar"), strings.HasPrefix(endpoint, "/clanwarleagues/wars/"):
		return time.Minute
	default:
		return 5 * time.Minute
	}
}

// Cada recurso tem uma única consulta em andamento, compartilhada entre usuários.
func (a *app) fetchClash(ctx context.Context, endpoint string) ([]byte, int, error) {
	if a.db != nil && a.db.postgres {
		return a.fetchClashShared(ctx, endpoint)
	}
	if err := ctx.Err(); err != nil {
		return nil, 502, fmt.Errorf("A consulta foi cancelada.")
	}
	a.cacheMu.Lock()
	if a.cache == nil {
		a.cache = make(map[string]clashCacheEntry)
		a.pending = make(map[string]*clashFlight)
	}
	now := time.Now()
	if entry, ok := a.cache[endpoint]; ok && now.Before(entry.expiresAt) {
		a.cacheMu.Unlock()
		return append([]byte(nil), entry.body...), entry.status, entry.err
	}
	if now.Before(a.retryAt) {
		a.cacheMu.Unlock()
		return nil, 429, fmt.Errorf("Limite de consultas da API atingido. Aguarde antes de atualizar novamente.")
	}
	flight, exists := a.pending[endpoint]
	if !exists {
		flight = &clashFlight{done: make(chan struct{})}
		a.pending[endpoint] = flight
		go func() {
			// Fechar uma aba não cancela a consulta usada pelos outros usuários.
			fetchCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			body, status, err := a.fetchClashUncached(fetchCtx, endpoint)
			fetchedAt := time.Now().UTC()
			ttl := clashTTL(endpoint)
			if err != nil {
				ttl = 30 * time.Second
			}
			entry := clashCacheEntry{body: body, status: status, err: err, fetchedAt: fetchedAt, expiresAt: fetchedAt.Add(ttl)}
			a.cacheMu.Lock()
			for key, old := range a.cache {
				if !fetchedAt.Before(old.expiresAt) {
					delete(a.cache, key)
				}
			}
			if len(a.cache) >= 128 {
				for key := range a.cache {
					delete(a.cache, key)
					break
				}
			}
			a.cache[endpoint] = entry
			flight.entry = entry
			delete(a.pending, endpoint)
			close(flight.done)
			a.cacheMu.Unlock()
		}()
	}
	a.cacheMu.Unlock()
	select {
	case <-ctx.Done():
		return nil, 502, fmt.Errorf("A consulta foi cancelada.")
	case <-flight.done:
		return append([]byte(nil), flight.entry.body...), flight.entry.status, flight.entry.err
	}
}

func (a *app) pauseClash(value string) {
	now := time.Now()
	until := now.Add(time.Minute)
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 && seconds <= 86400 {
		until = now.Add(time.Duration(seconds) * time.Second)
	} else if date, err := http.ParseTime(value); err == nil && date.After(now) {
		until = date
	}
	a.cacheMu.Lock()
	if until.After(a.retryAt) {
		a.retryAt = until
	}
	a.cacheMu.Unlock()
}

func (a *app) clashFetchedAt(endpoint string) time.Time {
	if a.db != nil && a.db.postgres {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var at int64
		if err := a.db.QueryRowContext(ctx, `SELECT fetched_at FROM crm_cache WHERE key=?`, a.cacheNamespace()+endpoint).Scan(&at); err == nil {
			return time.UnixMilli(at).UTC()
		}
		return time.Time{}
	}
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	if entry, ok := a.cache[endpoint]; ok {
		return entry.fetchedAt
	}
	return time.Now().UTC()
}
