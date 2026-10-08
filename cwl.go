package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

type leagueGroup struct {
	State  string `json:"state"`
	Season string `json:"season"`
	Clans  []struct {
		Tag string `json:"tag"`
	} `json:"clans"`
	Rounds []struct {
		WarTags []string `json:"warTags"`
	} `json:"rounds"`
}

func (a *app) fetchLeagueGroup(ctx context.Context, tag string) (json.RawMessage, leagueGroup, int, error) {
	body, status, err := a.fetchClash(ctx, "/clans/"+url.PathEscape("#"+tag)+"/currentwar/leaguegroup")
	var group leagueGroup
	if err != nil {
		return nil, group, status, err
	}
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, group, 502, fmt.Errorf("Resposta inválida da API do Clash para o grupo da liga.")
	}
	return json.RawMessage(body), group, status, nil
}

func (a *app) cwl(w http.ResponseWriter, r *http.Request) {
	tag, ok := a.selectedClanTag(w, r)
	if !ok {
		return
	}
	raw, _, status, err := a.fetchLeagueGroup(r.Context(), tag)
	if status == 404 {
		jsonResponse(w, 200, map[string]any{"available": false, "message": "Nenhum grupo da Liga de Guerra de Clãs está disponível. O clã pode não estar inscrito nesta temporada."})
		return
	}
	if err != nil {
		jsonResponse(w, status, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]any{"available": true, "group": raw})
}

func (a *app) cwlRound(w http.ResponseWriter, r *http.Request) {
	tag, ok := a.selectedClanTag(w, r)
	if !ok {
		return
	}
	index, err := strconv.Atoi(r.URL.Query().Get("round"))
	if err != nil || index < 1 {
		jsonResponse(w, 400, map[string]string{"error": "Escolha uma rodada válida da liga."})
		return
	}
	_, group, status, err := a.fetchLeagueGroup(r.Context(), tag)
	if err != nil {
		jsonResponse(w, status, map[string]string{"error": err.Error()})
		return
	}
	if index > len(group.Rounds) {
		jsonResponse(w, 400, map[string]string{"error": "Esta rodada não pertence ao grupo atual da liga."})
		return
	}
	tags := []string{}
	seen := map[string]bool{}
	for _, warTag := range group.Rounds[index-1].WarTags {
		warTag = strings.TrimSpace(warTag)
		if warTag == "" || warTag == "#0" || warTag == "0" || seen[warTag] {
			continue
		}
		seen[warTag] = true
		tags = append(tags, warTag)
	}
	if len(tags) == 0 {
		jsonResponse(w, 200, map[string]any{"pending": true, "message": "As guerras desta rodada ainda não foram definidas."})
		return
	}
	if len(tags) > 16 {
		jsonResponse(w, 502, map[string]string{"error": "Quantidade inválida de guerras nesta rodada da liga."})
		return
	}
	// Consulte apenas tags definidas pelo grupo da liga do clã selecionado.
	type result struct {
		body    json.RawMessage
		err     error
		matches bool
	}
	results := make([]result, len(tags))
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for i, warTag := range tags {
		wg.Add(1)
		go func(i int, warTag string) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
			case <-r.Context().Done():
				results[i].err = r.Context().Err()
				return
			}
			defer func() { <-slots }()
			body, _, fetchErr := a.fetchClash(r.Context(), "/clanwarleagues/wars/"+url.PathEscape("#"+normalizeTag(warTag)))
			if fetchErr != nil {
				results[i].err = fetchErr
				return
			}
			var sides struct {
				Clan struct {
					Tag string `json:"tag"`
				} `json:"clan"`
				Opponent struct {
					Tag string `json:"tag"`
				} `json:"opponent"`
			}
			if err := json.Unmarshal(body, &sides); err != nil {
				results[i].err = fmt.Errorf("Resposta inválida para a guerra da liga.")
				return
			}
			results[i].body = json.RawMessage(body)
			results[i].matches = normalizeTag(sides.Clan.Tag) == tag || normalizeTag(sides.Opponent.Tag) == tag
		}(i, warTag)
	}
	wg.Wait()
	for i, result := range results {
		if result.matches {
			jsonResponse(w, 200, map[string]any{"pending": false, "warTag": tags[i], "war": result.body})
			return
		}
	}
	for _, result := range results {
		if result.err != nil {
			jsonResponse(w, 502, map[string]string{"error": "Não foi possível carregar a guerra deste clã na liga. " + result.err.Error()})
			return
		}
	}
	jsonResponse(w, 200, map[string]any{"pending": false, "message": "Nenhuma guerra deste clã foi encontrada nesta rodada."})
}
