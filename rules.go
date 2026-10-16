package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

type ruleTopic struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Items []string `json:"items"`
}
type clanRulebook struct {
	ID                string      `json:"id"`
	Anchor            string      `json:"anchor"`
	Name              string      `json:"name"`
	WarSummary        string      `json:"warSummary"`
	InactivitySummary string      `json:"inactivitySummary"`
	DocumentURL       string      `json:"documentUrl"`
	Topics            []ruleTopic `json:"topics"`
}
type rulesDocument struct {
	Revision int64 `json:"revision"`
	General  struct {
		Title   string   `json:"title"`
		Penalty string   `json:"penalty"`
		Items   []string `json:"items"`
	} `json:"general"`
	Clans []clanRulebook `json:"clans"`
}

//go:embed rules_defaults.json
var rulesDefaults []byte

//go:embed rules_template.html
var rulesTemplate string

func decodeAdminJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		jsonResponse(w, 415, map[string]string{"error": "Envie os dados no formato JSON."})
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err == nil {
		err = decoder.Decode(&struct{}{})
		if err == io.EOF {
			return true
		}
	}
	jsonResponse(w, 400, map[string]string{"error": "Envie um único objeto JSON válido, com até 256 KiB."})
	return false
}

func validAdminText(value string, limit int, required bool) bool {
	return (!required || strings.TrimSpace(value) != "") && utf8.ValidString(value) && utf8.RuneCountInString(value) <= limit && strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) < 0
}

func validateRules(d *rulesDocument) error {
	bad := func() error { return fmt.Errorf("Preencha títulos, resumos e regras dentro dos limites indicados.") }
	if d.Revision < 1 || d.Revision == math.MaxInt64 || len(d.Clans) != 2 {
		return bad()
	}
	if !validAdminText(d.General.Title, 150, true) || !validAdminText(d.General.Penalty, 300, false) {
		return bad()
	}
	checkItems := func(items []string) bool {
		if len(items) < 1 || len(items) > 30 {
			return false
		}
		for _, s := range items {
			if !validAdminText(s, 1500, true) {
				return false
			}
		}
		return true
	}
	if !checkItems(d.General.Items) {
		return bad()
	}
	seen := map[string]bool{}
	for i := range d.Clans {
		c := &d.Clans[i]
		anchor := []string{"regras-principal", "regras-secundario"}[i]
		if c.ID != fmt.Sprint(i+1) || c.Anchor != anchor || !validAdminText(c.Name, 100, true) || !validAdminText(c.WarSummary, 200, true) || !validAdminText(c.InactivitySummary, 200, true) || len(c.Topics) < 1 || len(c.Topics) > 20 {
			return bad()
		}
		c.DocumentURL = strings.TrimSpace(c.DocumentURL)
		if c.DocumentURL != "" {
			u, err := url.Parse(c.DocumentURL)
			if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || !validAdminText(c.DocumentURL, 2048, true) {
				return fmt.Errorf("O link do regulamento deve ser HTTPS, sem usuário ou senha.")
			}
		}
		for _, t := range c.Topics {
			if !leadershipIDPattern.MatchString(t.ID) || !strings.HasPrefix(t.ID, anchor+"-") || seen[t.ID] || !validAdminText(t.Title, 150, true) || !checkItems(t.Items) {
				return bad()
			}
			seen[t.ID] = true
		}
	}
	return nil
}

func initRules(db *database) error {
	var defaults rulesDocument
	if err := json.Unmarshal(rulesDefaults, &defaults); err != nil {
		return err
	}
	if err := validateRules(&defaults); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if db.postgres {
		if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(629364820)`); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS public_rules(id INTEGER PRIMARY KEY CHECK(id=1), revision BIGINT NOT NULL, payload TEXT NOT NULL)`); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO public_rules(id,revision,payload) VALUES(1,1,?) ON CONFLICT(id) DO NOTHING`, string(rulesDefaults)); err != nil {
		return err
	}
	return tx.Commit()
}

func readRules(ctx context.Context, db *database) (rulesDocument, error) {
	var d rulesDocument
	var payload string
	var revision int64
	if err := db.QueryRowContext(ctx, `SELECT revision,payload FROM public_rules WHERE id=1`).Scan(&revision, &payload); err != nil {
		return d, err
	}
	err := json.Unmarshal([]byte(payload), &d)
	d.Revision = revision
	return d, err
}

func (a *app) rules(w http.ResponseWriter, r *http.Request) {
	d, err := readRules(r.Context(), a.db)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível carregar as regras."})
		return
	}
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(200)
		return
	}
	jsonResponse(w, 200, d)
}
func (a *app) saveRules(w http.ResponseWriter, r *http.Request) {
	var d rulesDocument
	if !decodeAdminJSON(w, r, &d) {
		return
	}
	if err := validateRules(&d); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	payload, _ := json.Marshal(d)
	result, err := a.db.ExecContext(r.Context(), `UPDATE public_rules SET revision=revision+1,payload=? WHERE id=1 AND revision=?`, string(payload), d.Revision)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível salvar as regras."})
		return
	}
	changed, err := result.RowsAffected()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível confirmar a gravação."})
		return
	}
	if changed != 1 {
		jsonResponse(w, 409, map[string]string{"error": "As regras foram alteradas em outra sessão. Suas edições foram mantidas; recarregue os dados antes de salvar novamente."})
		return
	}
	d.Revision++
	jsonResponse(w, 200, d)
}

func (a *app) renderHome(w http.ResponseWriter, r *http.Request) {
	d, err := readRules(r.Context(), a.db)
	if err != nil {
		http.Error(w, "Não foi possível carregar as regras. Tente novamente.", 503)
		return
	}
	source, err := fs.ReadFile(a.staticFS, "home.html")
	if err != nil {
		http.Error(w, "Página indisponível.", 500)
		return
	}
	tmpl, err := template.New("home").Funcs(template.FuncMap{"ruleNumber": func(i int) int { return i + 1 }}).Parse(string(source) + rulesTemplate)
	if err != nil {
		http.Error(w, "Página indisponível.", 500)
		return
	}
	var body bytes.Buffer
	if err = tmpl.ExecuteTemplate(&body, "home", d); err != nil {
		http.Error(w, "Página indisponível.", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodHead {
		w.Write(body.Bytes())
	}
}
