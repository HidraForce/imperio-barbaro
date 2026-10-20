package main

import (
	"net/http"
	"sort"
	"time"
)

const warningLifetime = 30 * 24 * time.Hour

var moderationSanctions = map[string]string{
	"two_wars":  "Fora das próximas duas guerras.",
	"demotion":  "Perda de cargo.",
	"expulsion": "Expulsão do clã.",
}

func automaticSector(source string) string {
	switch source {
	case "war", "cwl":
		return "wars"
	case "raids":
		return "capital"
	}
	return ""
}

func validModerationSector(sector string) bool {
	return sector == "games" || sector == "capital" || sector == "wars" || sector == "replays"
}

func warningIssuedAt(record moderationRecord) (time.Time, error) {
	issued := record.IssuedAt
	if issued == "" {
		issued = record.CreatedAt
	}
	return time.Parse(time.RFC3339Nano, issued)
}

// Expiry is derived from the history, without deleting records or reviving a
// cycle that already expired. Only confirmed warnings extend the current cycle.
func evaluateModeration(records []moderationRecord, now time.Time) []moderationRecord {
	result := append([]moderationRecord{}, records...)
	byPlayer := map[string][]int{}
	for i := range result {
		r := &result[i]
		r.ExpiresAt = ""
		if r.Sector == "" {
			r.Sector = automaticSector(r.Source)
		}
		if r.Kind == "warning" && r.Status == "active" && validModerationSector(r.Sector) {
			byPlayer[normalizeTag(r.PlayerTag)] = append(byPlayer[normalizeTag(r.PlayerTag)], i)
		}
	}
	for _, indices := range byPlayer {
		sort.SliceStable(indices, func(i, j int) bool {
			a, _ := warningIssuedAt(result[indices[i]])
			b, _ := warningIssuedAt(result[indices[j]])
			return a.Before(b)
		})
		cycle := []int{}
		var expires time.Time
		finish := func() {
			for _, index := range cycle {
				result[index].ExpiresAt = expires.UTC().Format(time.RFC3339Nano)
			}
		}
		for _, index := range indices {
			issued, err := warningIssuedAt(result[index])
			if err != nil || issued.After(now) {
				continue
			}
			if !issued.Before(expires) {
				finish()
				cycle = nil
			}
			cycle = append(cycle, index)
			expires = issued.Add(warningLifetime)
		}
		finish()
	}
	return result
}

func activeModerationWarning(record moderationRecord, now time.Time) bool {
	expires, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	return record.Kind == "warning" && record.Status == "active" && validModerationSector(record.Sector) && err == nil && now.Before(expires)
}

func moderationDegree(records []moderationRecord, playerTag string, now time.Time) int {
	count := 0
	sectors := map[string]int{}
	for _, r := range records {
		if normalizeTag(r.PlayerTag) == normalizeTag(playerTag) && activeModerationWarning(r, now) {
			count++
			sectors[r.Sector]++
		}
	}
	if len(sectors) == 4 {
		return 4
	}
	for _, count := range sectors {
		if count >= 4 {
			return 4
		}
	}
	return min(count, 3)
}

func (a *app) validateModerationPolicy(w http.ResponseWriter, r *http.Request, tag string, record, saved *moderationRecord) bool {
	reject := func(message string) bool {
		jsonResponse(w, 400, map[string]string{"error": message})
		return false
	}
	if record.Kind == "warning" {
		if (record.Status == "active" || record.Status == "pending") && !validModerationSector(record.Sector) {
			return reject("Selecione o setor do aviso: Jogos, Capital, Guerras ou Replays.")
		}
		record.Action, record.Sanction = "", ""
		return true
	}
	// Historical decisions may still be justified, closed or removed even when
	// their supporting warnings have since expired.
	if record.Status != "active" && record.Status != "pending" {
		return true
	}
	unchanged := saved != nil && saved.Kind == "punishment" && saved.Status == record.Status && saved.PlayerTag == record.PlayerTag && saved.Sanction == record.Sanction && saved.Action == record.Action
	if unchanged {
		return true
	}
	if _, ok := moderationSanctions[record.Sanction]; !ok {
		return reject("Escolha a medida: fora de duas guerras, perda de cargo ou expulsão.")
	}
	if record.Status == "pending" {
		return true
	}
	records, err := readModerationRecords(r.Context(), a.db, tag)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível conferir os avisos antes da punição."})
		return false
	}
	// Converting an existing warning must not let that same warning count as
	// evidence for its replacement punishment.
	remaining := records[:0]
	for _, item := range records {
		if item.ID != record.ID {
			remaining = append(remaining, item)
		}
	}
	now := time.Now().UTC()
	degree := moderationDegree(evaluateModeration(remaining, now), record.PlayerTag, now)
	if degree < 2 {
		return reject("Todo membro recebe um aviso antes da punição. São necessários pelo menos dois avisos ativos.")
	}
	if record.Sanction == "expulsion" && degree != 4 {
		return reject("Expulsão exige quatro avisos ativos no mesmo setor ou pelo menos um em cada um dos quatro setores.")
	}
	if degree == 4 && record.Sanction != "expulsion" {
		return reject("O membro atingiu o grau 4. A medida prevista pela regra é expulsão.")
	}
	record.AppliedAt = now.Format(time.RFC3339Nano)
	return true
}
