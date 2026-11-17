package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

const editorSessionHeader = "X-Editor-Session"

func (a *app) editorAccessRoutes(r chi.Router, leadershipPassword, rulesPassword string) error {
	for _, editor := range []struct {
		name, envName, password string
		read, save              http.HandlerFunc
	}{
		{"leadership", "LEADERSHIP", leadershipPassword, a.leadership, a.saveLeadership},
		{"rules", "RULES_PASS", rulesPassword, a.rules, a.saveRules},
	} {
		vault := newScopedVault(editor.password, "editor-"+editor.name, editorSessionHeader, editor.envName)
		if a.db.postgres && vault.configured {
			if err := vault.useSharedStore(a.db, editor.password); err != nil {
				return err
			}
		}
		path := "/api/" + editor.name
		r.Post(path+"/unlock", vault.unlock)
		r.Delete(path+"/session", vault.lock)
		r.With(vault.protect).Get(path, editor.read)
		r.With(vault.protect).Put(path, editor.save)
	}
	return nil
}
