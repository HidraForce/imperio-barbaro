package main

import "net/http"

func (a *app) contactCoverage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	rows, err := a.db.QueryContext(r.Context(), `SELECT a.player_tag,CASE WHEN TRIM(p.phone)<>'' THEN 1 ELSE 0 END FROM contact_accounts a JOIN contact_people p ON p.id=a.contact_id`)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível conferir os números cadastrados."})
		return
	}
	defer rows.Close()
	items := map[string]bool{}
	for rows.Next() {
		var tag string
		var registered int
		if err := rows.Scan(&tag, &registered); err != nil {
			jsonResponse(w, 500, map[string]string{"error": "Não foi possível ler a situação dos contatos."})
			return
		}
		items[tag] = registered == 1
	}
	if rows.Err() != nil {
		jsonResponse(w, 500, map[string]string{"error": "A consulta dos contatos não foi concluída."})
		return
	}
	jsonResponse(w, 200, map[string]any{"items": items})
}
