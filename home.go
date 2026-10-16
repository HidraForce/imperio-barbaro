package main

import (
	"io/fs"
	"net"
	"net/http"
	"strings"
)

func publicHome(assets fs.FS, dashboard http.Handler, render ...http.HandlerFunc) http.Handler {
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
		if r.URL.Path == "/" && len(render) > 0 {
			render[0](w, r)
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
		}[r.URL.Path]
		if name == "" {
			http.NotFound(w, r)
			return
		}
		switch name {
		case "imperio-hero-768.webp", "imperio-hero-1536.webp":
			w.Header().Set("Content-Type", "image/webp")
		}
		http.ServeFileFS(w, r, assets, name)
	})
}
