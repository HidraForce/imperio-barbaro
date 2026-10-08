package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type clashTransport func(*http.Request) (*http.Response, error)

func (f clashTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func testClash(fn clashTransport) *app {
	return &app{token: "token-de-teste", clash: &http.Client{Transport: fn}}
}
func clashResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestCacheSharesConcurrentRequests(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	a := testClash(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return clashResponse(200, `{"name":"Clã"}`), nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, status, err := a.fetchClash(context.Background(), "/clans/%23ABC")
			if err != nil || status != 200 || string(body) != `{"name":"Clã"}` {
				t.Errorf("resposta: %s, %d, %v", body, status, err)
			}
		}()
	}
	<-started
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("consultas externas: %d", calls.Load())
	}
	body, _, _ := a.fetchClash(context.Background(), "/clans/%23ABC")
	body[0] = 'X'
	body, _, _ = a.fetchClash(context.Background(), "/clans/%23ABC")
	if body[0] != '{' {
		t.Fatal("o consumidor alterou o cache compartilhado")
	}
}

func TestCacheExpirationAndClanIsolation(t *testing.T) {
	var calls atomic.Int32
	a := testClash(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return clashResponse(200, `{"tag":"`+r.URL.Path+`"}`), nil
	})
	first, _, _ := a.fetchClash(context.Background(), "/clans/%23ABC")
	second, _, _ := a.fetchClash(context.Background(), "/clans/%23DEF")
	if string(first) == string(second) || calls.Load() != 2 {
		t.Fatal("os clãs devem ter entradas distintas")
	}
	a.cacheMu.Lock()
	entry := a.cache["/clans/%23ABC"]
	entry.expiresAt = time.Now().Add(-time.Second)
	a.cache["/clans/%23ABC"] = entry
	a.cacheMu.Unlock()
	a.fetchClash(context.Background(), "/clans/%23ABC")
	if calls.Load() != 3 {
		t.Fatal("cache expirado não foi renovado")
	}
}

func TestRateLimitPausesOtherEndpoints(t *testing.T) {
	for _, retry := range []string{"120", time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat), "inválido"} {
		t.Run(retry, func(t *testing.T) {
			var calls atomic.Int32
			a := testClash(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				resp := clashResponse(429, `{}`)
				resp.Header.Set("Retry-After", retry)
				return resp, nil
			})
			_, status, err := a.fetchClash(context.Background(), "/clans/%23ABC")
			if status != 429 || err == nil {
				t.Fatal("limite não foi informado")
			}
			_, status, err = a.fetchClash(context.Background(), "/clans/%23DEF/currentwar")
			if status != 429 || err == nil || calls.Load() != 1 {
				t.Fatal("a pausa deve valer para toda a API")
			}
			a.cacheMu.Lock()
			a.retryAt = time.Now().Add(-time.Second)
			a.cacheMu.Unlock()
			a.fetchClash(context.Background(), "/clans/%23DEF/currentwar")
			if calls.Load() != 2 {
				t.Fatal("as consultas não foram retomadas após a pausa")
			}
		})
	}
}

func TestCanceledCallerDoesNotCancelSharedFetch(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	a := testClash(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-release
		if r.Context().Err() != nil {
			t.Error("consulta compartilhada foi cancelada")
		}
		return clashResponse(200, `{}`), nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, err := a.fetchClash(ctx, "/clans/%23ABC")
		if err == nil {
			t.Error("cancelamento não informado")
		}
	}()
	<-started
	cancel()
	<-done
	close(release)
	_, status, err := a.fetchClash(context.Background(), "/clans/%23ABC")
	if status != 200 || err != nil {
		t.Fatalf("consulta de outro usuário falhou: %v", err)
	}
}

func TestErrorsAreCachedBriefly(t *testing.T) {
	for _, status := range []int{403, 404, 503, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			a := testClash(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				return clashResponse(status, "resposta inválida"), nil
			})
			for i := 0; i < 2; i++ {
				_, _, err := a.fetchClash(context.Background(), "/clans/%23ABC")
				if err == nil {
					t.Fatal("resposta inválida foi aceita")
				}
			}
			if calls.Load() != 1 {
				t.Fatal("erro repetido consumiu outra consulta")
			}
			entry := a.cache["/clans/%23ABC"]
			if entry.expiresAt.Sub(entry.fetchedAt) != 30*time.Second {
				t.Fatal("erro deve expirar em 30 segundos")
			}
		})
	}
}

func TestCacheTTLs(t *testing.T) {
	for endpoint, want := range map[string]time.Duration{"/clans/%23ABC": 5 * time.Minute, "/clans/%23ABC/currentwar": time.Minute, "/clans/%23ABC/currentwar/leaguegroup": time.Minute, "/clanwarleagues/wars/%23WAR": time.Minute, "/clans/%23ABC/capitalraidseasons?limit=10": 15 * time.Minute} {
		if got := clashTTL(endpoint); got != want {
			t.Errorf("%s: %v, esperado %v", endpoint, got, want)
		}
	}
}
