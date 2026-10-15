package main

import (
	"io/fs"
	"net"
	"net/http"
	"strings"
)

// Only these public hosts bypass the dashboard handler, and only for an
// explicit set of public files. The dashboard's authentication stays intact.
func publicHome(assets fs.FS, dashboard http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.ToLower(host)
		if host != "imperiobarbaro.com.br" && host != "www.imperiobarbaro.com.br" {
			dashboard.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		name := map[string]string{
			"/":                       "home.html",
			"/home.js":                "home.js",
			"/home.css":               "home.css",
			"/icon.svg":               "icon.svg",
			"/sw.js":                  "sw.js",
			"/imperio-hero-768.webp":  "imperio-hero-768.webp",
			"/imperio-hero-1536.webp": "imperio-hero-1536.webp",
			"/regras-principal.pdf":   "regras-principal.pdf",
			"/regras-secundario.pdf":  "regras-secundario.pdf",
		}[r.URL.Path]
		if name == "" {
			http.NotFound(w, r)
			return
		}
		switch name {
		case "imperio-hero-768.webp", "imperio-hero-1536.webp":
			w.Header().Set("Content-Type", "image/webp")
		case "regras-principal.pdf", "regras-secundario.pdf":
			w.Header().Set("Content-Type", "application/pdf")
		}
		http.ServeFileFS(w, r, assets, name)
	})
}
