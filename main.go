package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "modernc.org/sqlite"
)

//go:embed web/dist/* web/dist/assets/*
var webFiles embed.FS

type app struct {
	db            *sql.DB
	clash         *http.Client
	token         string
	clanTags      map[string]string
	staticFS      fs.FS
	contactsVault *contactVault
	cacheMu       sync.Mutex
	cache         map[string]clashCacheEntry
	pending       map[string]*clashFlight
	retryAt       time.Time
}

func main() {
	username, password := os.Getenv("APP_USERNAME"), os.Getenv("APP_PASSWORD")
	if !validCredentials(username, password) {
		log.Fatal("Defina APP_USERNAME e uma APP_PASSWORD privada no .env antes de iniciar o painel")
	}
	contactsPassword := os.Getenv("CONTACTS_PASSWORD")
	if contactsPassword != "" && (!validCredentials("contatos", contactsPassword) || contactsPassword == password) {
		log.Fatal("CONTACTS_PASSWORD deve ter pelo menos 16 caracteres e ser diferente de APP_PASSWORD.")
	}
	path := env("DATABASE_PATH", "./data/clan.db")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		log.Fatal(err)
	}
	if err := initContacts(db); err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := initRaids(db); err != nil {
		log.Fatal(err)
	}

	assets, err := fs.Sub(webFiles, "web/dist")
	if err != nil {
		log.Fatal(err)
	}
	a := &app{
		db:            db,
		clash:         &http.Client{Timeout: 12 * time.Second},
		token:         os.Getenv("CLASH_API_TOKEN"),
		clanTags:      map[string]string{"1": normalizeTag(os.Getenv("CLAN_TAG1")), "2": normalizeTag(os.Getenv("CLAN_TAG2"))},
		staticFS:      assets,
		contactsVault: newContactVault(contactsPassword),
	}
	r := chi.NewRouter()
	r.Use(secureHeaders)
	r.Use(middleware.RequestID, middleware.Logger, middleware.Recoverer)
	r.Use(newAccessGuard(username, password).middleware)
	r.Use(sameOriginAPI)
	r.Get("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/api/config", a.config)
	r.Get("/api/clan", a.clan)
	r.Get("/api/war", a.war)
	r.Get("/api/raids", a.raids)
	r.Get("/api/cwl", a.cwl)
	r.Get("/api/cwl/round", a.cwlRound)
	r.Post("/api/contacts/unlock", a.contactsVault.unlock)
	r.Delete("/api/contacts/session", a.contactsVault.lock)
	r.Group(func(r chi.Router) {
		r.Use(a.contactsVault.protect)
		r.Get("/api/contacts", a.contacts)
		r.Post("/api/contacts", a.saveContact)
		r.Post("/api/contacts/merge", a.mergeContact)
		r.Delete("/api/contacts/{id}", a.deleteContact)
	})
	r.Get("/*", a.static)

	port := env("PORT", "2025")
	log.Printf("Painel disponível na porta :%s", port)
	log.Fatal(secureServer(":"+port, r).ListenAndServe())
}

func (a *app) config(w http.ResponseWriter, _ *http.Request) {
	clans := []map[string]any{}
	for _, id := range []string{"1", "2"} {
		tag := a.clanTags[id]
		clans = append(clans, map[string]any{"id": id, "tag": tag, "configured": tag != ""})
	}
	jsonResponse(w, http.StatusOK, map[string]any{"configured": a.token != "" && (a.clanTags["1"] != "" || a.clanTags["2"] != ""), "clans": clans, "contactsConfigured": a.contactsVault != nil && a.contactsVault.configured})
}

func (a *app) clan(w http.ResponseWriter, r *http.Request) {
	tag, ok := a.selectedClanTag(w, r)
	if ok {
		a.clashGET(w, r, tag, "/clans/"+url.PathEscape("#"+tag))
	}
}

func (a *app) war(w http.ResponseWriter, r *http.Request) {
	tag, ok := a.selectedClanTag(w, r)
	if ok {
		a.clashGET(w, r, tag, "/clans/"+url.PathEscape("#"+tag)+"/currentwar")
	}
}

func (a *app) selectedClanTag(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.URL.Query().Get("clan")
	if id == "" {
		id = "1"
		if a.clanTags[id] == "" {
			id = "2"
		}
	}
	tag, validSlot := a.clanTags[id]
	if !validSlot {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "Escolha o clã 1 ou o clã 2."})
		return "", false
	}
	if tag == "" {
		jsonResponse(w, http.StatusServiceUnavailable, map[string]string{"error": "Este clã não tem uma tag configurada no .env."})
		return "", false
	}
	return tag, true
}

func (a *app) clashGET(w http.ResponseWriter, r *http.Request, clanTag, endpoint string) {
	if a.token == "" || clanTag == "" {
		jsonResponse(w, http.StatusServiceUnavailable, map[string]string{"error": "Defina CLASH_API_TOKEN e CLAN_TAG1 ou CLAN_TAG2 no .env e reinicie o aplicativo."})
		return
	}
	body, status, err := a.fetchClash(r.Context(), endpoint)
	if err != nil {
		jsonResponse(w, status, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (a *app) contacts(w http.ResponseWriter, r *http.Request) {
	items, err := listContacts(r.Context(), a.db)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível carregar os contatos."})
		return
	}
	jsonResponse(w, 200, items)
}

func (a *app) saveContact(w http.ResponseWriter, r *http.Request) {
	var c contact
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		jsonResponse(w, 400, map[string]string{"error": "Dados de contato inválidos."})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		jsonResponse(w, 400, map[string]string{"error": "Envie apenas um contato por consulta."})
		return
	}
	if err := validateContact(&c); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := writeContact(r.Context(), a.db, &c); err != nil {
		status, message := 500, "Não foi possível salvar o contato."
		if errors.Is(err, errAccountInUse) {
			status, message = 409, err.Error()
		}
		if errors.Is(err, errContactNotFound) {
			status, message = 404, err.Error()
		}
		jsonResponse(w, status, map[string]string{"error": message})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) deleteContact(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		jsonResponse(w, 400, map[string]string{"error": "Identificador de contato inválido."})
		return
	}
	if err := eraseContact(r.Context(), a.db, id); err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível excluir o contato."})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) mergeContact(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TargetID int64 `json:"targetId"`
		SourceID int64 `json:"sourceId"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		jsonResponse(w, 400, map[string]string{"error": "Dados inválidos para reunir contatos."})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || input.TargetID <= 0 || input.SourceID <= 0 || input.TargetID == input.SourceID {
		jsonResponse(w, 400, map[string]string{"error": "Escolha dois contatos diferentes da mesma pessoa."})
		return
	}
	if err := mergeContacts(r.Context(), a.db, input.TargetID, input.SourceID); err != nil {
		status, message := 500, "Não foi possível reunir os contatos."
		if errors.Is(err, errContactNotFound) {
			status, message = 404, err.Error()
		}
		if errors.Is(err, errMergeTooMany) || errors.Is(err, errMergeNotes) || errors.Is(err, errInvalidMerge) {
			status, message = 400, err.Error()
		}
		jsonResponse(w, status, map[string]string{"error": message})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) static(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	if strings.HasPrefix(name, "api/") {
		http.NotFound(w, r)
		return
	}
	if _, err := fs.Stat(a.staticFS, name); err != nil {
		if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
	}
	if name == "index.html" || name == "sw.js" {
		w.Header().Set("Cache-Control", "no-store")
	}
	clone := r.Clone(r.Context())
	clone.URL.Path = "/" + name
	if name == "index.html" {
		clone.URL.Path = "/"
	}
	http.FileServer(http.FS(a.staticFS)).ServeHTTP(w, clone)
}

func normalizeTag(tag string) string { return strings.TrimPrefix(strings.TrimSpace(tag), "#") }
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
