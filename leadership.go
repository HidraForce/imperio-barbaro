package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type leadershipItem struct {
	ID          string `json:"id"`
	ClanID      string `json:"clanId"`
	Role        string `json:"role"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type leadershipDocument struct {
	Revision int64            `json:"revision"`
	Items    []leadershipItem `json:"items"`
}

//go:embed leadership_defaults.json
var leadershipDefaults []byte

var leadershipIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func initLeadership(db *database) error {
	var defaults leadershipDocument
	if err := json.Unmarshal(leadershipDefaults, &defaults); err != nil {
		return err
	}
	if err := validateLeadership(&defaults); err != nil {
		return err
	}
	payload, err := json.Marshal(defaults.Items)
	if err != nil {
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
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS public_leadership (
		id INTEGER PRIMARY KEY CHECK(id = 1),
		revision BIGINT NOT NULL CHECK(revision > 0),
		payload TEXT NOT NULL
	)`); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO public_leadership(id, revision, payload) VALUES(1, ?, ?) ON CONFLICT(id) DO NOTHING`, defaults.Revision, string(payload)); err != nil {
		return err
	}
	return tx.Commit()
}

func validateLeadership(document *leadershipDocument) error {
	if document.Revision < 1 || document.Revision == math.MaxInt64 {
		return fmt.Errorf("Versão da liderança inválida. Recarregue a página.")
	}
	if document.Items == nil || len(document.Items) > 40 {
		return fmt.Errorf("Envie a lista de liderança com até 40 funções.")
	}
	seen := make(map[string]bool, len(document.Items))
	for i := range document.Items {
		item := &document.Items[i]
		item.ID = strings.TrimSpace(item.ID)
		item.ClanID = strings.TrimSpace(item.ClanID)
		item.Role = strings.TrimSpace(item.Role)
		item.Name = strings.TrimSpace(item.Name)
		item.Description = strings.TrimSpace(item.Description)
		if !leadershipIDPattern.MatchString(item.ID) || seen[item.ID] {
			return fmt.Errorf("Cada função deve ter um identificador único de até 64 letras, números, hífens ou sublinhados.")
		}
		seen[item.ID] = true
		if item.ClanID != "1" && item.ClanID != "2" {
			return fmt.Errorf("Escolha o clã principal ou secundário para cada função.")
		}
		if item.Role == "" || utf8.RuneCountInString(item.Role) > 100 || utf8.RuneCountInString(item.Name) > 100 || item.Description == "" || utf8.RuneCountInString(item.Description) > 600 {
			return fmt.Errorf("Informe a função (até 100 caracteres) e a descrição (até 600). O nome é opcional e aceita até 100 caracteres.")
		}
		for field, value := range []string{item.Role, item.Name, item.Description} {
			if strings.IndexFunc(value, func(r rune) bool {
				return unicode.IsControl(r) && !(field == 2 && (r == '\n' || r == '\r' || r == '\t'))
			}) >= 0 {
				return fmt.Errorf("Os textos da liderança não podem conter caracteres de controle.")
			}
		}
	}
	return nil
}

func readLeadership(ctx context.Context, db *database) (leadershipDocument, error) {
	var document leadershipDocument
	var payload string
	if err := db.QueryRowContext(ctx, `SELECT revision, payload FROM public_leadership WHERE id=1`).Scan(&document.Revision, &payload); err != nil {
		return document, err
	}
	err := json.Unmarshal([]byte(payload), &document.Items)
	return document, err
}

func (a *app) leadership(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		jsonResponse(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido."})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	document, err := readLeadership(r.Context(), a.db)
	if err != nil {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível carregar a liderança."})
		return
	}
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		return
	}
	jsonResponse(w, http.StatusOK, document)
}

func (a *app) saveLeadership(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", "PUT")
		jsonResponse(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido."})
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		jsonResponse(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Envie os dados no formato JSON."})
		return
	}
	var document leadershipDocument
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&document); err == nil {
		if trailing := decoder.Decode(&struct{}{}); trailing != io.EOF {
			if trailing == nil {
				trailing = errors.New("more than one JSON value")
			}
			err = trailing
		}
	}
	if err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		jsonResponse(w, status, map[string]string{"error": "Envie um único objeto de liderança válido (até 64 KiB)."})
		return
	}
	if err = validateLeadership(&document); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	payload, err := json.Marshal(document.Items)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível salvar a liderança."})
		return
	}
	// A single conditional UPDATE keeps the list and revision atomic on both databases.
	result, err := a.db.ExecContext(r.Context(), `UPDATE public_leadership SET payload=?, revision=revision+1 WHERE id=1 AND revision=?`, string(payload), document.Revision)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível salvar a liderança."})
		return
	}
	updated, err := result.RowsAffected()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível confirmar a atualização da liderança."})
		return
	}
	if updated != 1 {
		jsonResponse(w, http.StatusConflict, map[string]string{"error": "A liderança foi alterada em outra sessão. Recarregue antes de salvar novamente."})
		return
	}
	document.Revision++
	w.Header().Set("Cache-Control", "no-store")
	jsonResponse(w, http.StatusOK, document)
}

func (a *app) publicLeadership(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/public/leadership" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			a.leadership(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
