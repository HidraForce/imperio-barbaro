package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"testing/fstest"
)

func TestPublicHomeBinaryAssets(t *testing.T) {
	content := []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0xff, 0x80, 0x01}
	assets := fstest.MapFS{"private.webp": {Data: content}, "private.pdf": {Data: content}}
	files := map[string]string{
		"imperio-hero-768.webp":  "image/webp",
		"imperio-hero-1536.webp": "image/webp",
		"regras-principal.pdf":   "application/pdf",
		"regras-secundario.pdf":  "application/pdf",
	}
	for name := range files {
		assets[name] = &fstest.MapFile{Data: content}
	}
	guard := newAccessGuard("leader", "a-long-private-test-password")
	handler := secureHeaders(publicHome(assets, guard.middleware(http.NotFoundHandler())))
	for name, contentType := range files {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(name+method, func(t *testing.T) {
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, httptest.NewRequest(method, "https://imperiobarbaro.com.br/"+name, nil))
				if res.Code != http.StatusOK || res.Header().Get("Content-Type") != contentType {
					t.Fatalf("got status %d, content type %q", res.Code, res.Header().Get("Content-Type"))
				}
				if res.Header().Get("Content-Length") != strconv.Itoa(len(content)) {
					t.Fatal("incorrect binary content length")
				}
				if method == http.MethodGet && !bytes.Equal(res.Body.Bytes(), content) {
					t.Fatal("binary content changed")
				}
				if method == http.MethodHead && res.Body.Len() != 0 {
					t.Fatal("HEAD returned body")
				}
			})
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "https://dashboard.imperiobarbaro.com.br/"+name, nil))
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("dashboard asset %s bypassed authentication: %d", name, res.Code)
		}
	}
	for _, name := range []string{"private.webp", "private.pdf", "assets/imperio-hero-768.webp", "imperio-hero-3072.webp"} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "https://imperiobarbaro.com.br/"+name, nil))
		if res.Code != http.StatusNotFound {
			t.Fatalf("non-allowlisted asset %s returned %d", name, res.Code)
		}
	}
}

func TestPublicHomeKeepsDashboardProtected(t *testing.T) {
	assets := fstest.MapFS{"home.html": {Data: []byte("public home")}, "home.css": {Data: []byte("body{}")}, "index.html": {Data: []byte("private dashboard")}}
	guard := newAccessGuard("leader", "a-long-private-test-password")
	handler := secureHeaders(publicHome(assets, guard.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("private dashboard")) }))))
	for _, tc := range []struct {
		host, path, method string
		auth               bool
		status             int
	}{
		{"imperiobarbaro.com.br", "/", "GET", false, 200},
		{"www.imperiobarbaro.com.br", "/", "HEAD", false, 200},
		{"IMPERIOBARBARO.COM.BR:443", "/home.css", "GET", false, 200},
		{"imperiobarbaro.com.br", "/api/contacts", "GET", false, 404},
		{"imperiobarbaro.com.br", "/api/health", "GET", true, 404},
		{"imperiobarbaro.com.br", "/index.html", "GET", false, 404},
		{"imperiobarbaro.com.br", "/assets/private.js", "GET", false, 404},
		{"imperiobarbaro.com.br", "/", "POST", false, 405},
		{"dashboard.imperiobarbaro.com.br", "/", "GET", false, 401},
		{"dashboard.imperiobarbaro.com.br", "/api/contacts", "GET", false, 401},
		{"dashboard.imperiobarbaro.com.br", "/", "GET", true, 200},
		{"localhost:2025", "/", "GET", false, 401},
		{"imperiobarbaro.com.br.evil.test", "/", "GET", false, 401},
	} {
		t.Run(tc.host+tc.path+tc.method, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "https://"+tc.host+tc.path, nil)
			req.Header.Set("X-Forwarded-Host", "imperiobarbaro.com.br")
			if tc.auth {
				req.SetBasicAuth("leader", "a-long-private-test-password")
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != tc.status {
				t.Fatalf("got %d, want %d: %s", res.Code, tc.status, res.Body.String())
			}
			if res.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing no-store")
			}
			if tc.method == "HEAD" && res.Body.Len() != 0 {
				t.Fatal("HEAD returned body")
			}
		})
	}
}
