package main

import (
	"io/fs"
	"net"
	"net/http"
	"strings"
)

func publicHome(assets fs.FS, dashboard http.Handler, render ...http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The member app has an explicit public allowlist, including on localhost
		// for previews. No dashboard assets or management APIs are exposed here.
		pwa := map[string]string{"/instalar": "install.html", "/app": "member.html", "/member.js": "member.js", "/member.css": "member.css", "/member.webmanifest": "member.webmanifest", "/pwa-sw.js": "pwa-sw.js", "/pwa-state.js": "pwa-state.js", "/pwa-state-worker.js": "pwa-state-worker.js", "/app-icon-192.png": "app-icon-192.png", "/app-icon-512.png": "app-icon-512.png", "/app-icon-180.png": "app-icon-180.png", "/notification-badge.png": "notification-badge.png"}[r.URL.Path]
		if pwa != "" {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				w.WriteHeader(405)
				return
			}
			if pwa == "member.webmanifest" {
				w.Header().Set("Content-Type", "application/manifest+json")
			}
			http.ServeFileFS(w, r, assets, pwa)
			return
		}
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
