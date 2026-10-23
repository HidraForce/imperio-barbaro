package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func intPointer(n int) *int { return &n }

func TestFinalWarRequiresClosedEventAndCompleteAttackCounts(t *testing.T) {
	now := time.Now().UTC()
	since := now.Add(-7 * 24 * time.Hour)
	base := moderationWar{State: "warEnded", TeamSize: 2, StartTime: now.Add(-25 * time.Hour).Format("20060102T150405.000Z"), EndTime: now.Add(-time.Hour).Format("20060102T150405.000Z"), AttacksPerMember: 2, Clan: moderationSide{Tag: "#P0Y8LQ", Attacks: intPointer(1), Members: []moderationMember{{Tag: "#P0Y8LQ", Name: "Zero"}, {Tag: "#QY8L0P", Name: "Parcial", Attacks: []json.RawMessage{json.RawMessage(`{}`)}}}}}
	cases := []struct {
		name   string
		change func(*moderationWar)
	}{
		{"still in progress", func(w *moderationWar) { w.State = "inWar" }},
		{"future end", func(w *moderationWar) { w.EndTime = now.Add(time.Hour).Format("20060102T150405.000Z") }},
		{"missing start", func(w *moderationWar) { w.StartTime = "" }},
		{"invalid interval", func(w *moderationWar) { w.StartTime = w.EndTime }},
		{"unknown lineup size", func(w *moderationWar) { w.TeamSize = 0 }},
		{"truncated lineup", func(w *moderationWar) { w.TeamSize = 3 }},
		{"missing aggregate", func(w *moderationWar) { w.Clan.Attacks = nil }},
		{"attack arrays incomplete", func(w *moderationWar) { w.Clan.Attacks = intPointer(2) }},
		{"normal war limit missing", func(w *moderationWar) { w.AttacksPerMember = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			war := base
			tc.change(&war)
			body, _ := json.Marshal(war)
			if got := warWarnings(body, "P0Y8LQ", "war", war.EndTime, since, now); len(got) != 0 {
				t.Fatal("unconfirmed event generated warnings", got)
			}
		})
	}
	body, _ := json.Marshal(base)
	if got := warWarnings(body, "P0Y8LQ", "war", base.EndTime, since, now); len(got) != 1 || got[0].PlayerName != "Zero" {
		t.Fatal("partial attack became a warning", got)
	}
	// CWL omits attacksPerMember, but still has one attack per selected player.
	base.AttacksPerMember = 0
	body, _ = json.Marshal(base)
	if got := warWarnings(body, "P0Y8LQ", "cwl", "league-war", since, now); len(got) != 1 || got[0].PlayerName != "Zero" {
		t.Fatal("CWL rule not respected", got)
	}
}

func TestFinalRaidsIncludeEveryUnusedAvailableAttackWithoutInventingBonus(t *testing.T) {
	now := time.Now().UTC()
	since := now.Add(-7 * 24 * time.Hour)
	season := moderationRaidSeason{State: "ended", StartTime: now.Add(-3 * 24 * time.Hour).Format("20060102T150405.000Z"), EndTime: now.Add(-time.Hour).Format("20060102T150405.000Z"), TotalAttacks: intPointer(13), Members: []moderationRaidMember{
		{Tag: "#P0Y8LQ", Name: "Zero", Attacks: intPointer(0), AttackLimit: intPointer(5)},
		{Tag: "#QY8L0P", Name: "Parcial", Attacks: intPointer(3), AttackLimit: intPointer(5)},
		{Tag: "#QQQQ", Name: "Bônus pendente", Attacks: intPointer(5), AttackLimit: intPointer(5), Bonus: 1},
		{Tag: "#PPPP", Name: "Sem bônus liberado", Attacks: intPointer(5), AttackLimit: intPointer(5)},
	}}
	get := func(s moderationRaidSeason, roster map[string][]raidRosterMember) []moderationRecord {
		body, _ := json.Marshal(map[string]any{"items": []moderationRaidSeason{s}})
		return raidWarnings(body, roster, since, now)
	}
	roster := map[string][]raidRosterMember{season.StartTime: {{Tag: "#LLLL", Name: "Ausente na lista final"}, {Tag: "#QY8L0P", Name: "Parcial"}}}
	got := get(season, roster)
	if len(got) != 4 {
		t.Fatalf("wrong incomplete participants: %+v", got)
	}
	for _, record := range got {
		if record.PlayerName == "Sem bônus liberado" || record.Status != "pending" {
			t.Fatal("invented bonus or applied unreviewed punishment", record)
		}
	}
	for _, state := range []string{"ongoing", "", "unknown"} {
		s := season
		s.State = state
		if len(get(s, roster)) != 0 {
			t.Fatal("unfinished raid generated warnings", state)
		}
	}
	for _, change := range []func(*moderationRaidSeason){
		func(s *moderationRaidSeason) { s.EndTime = now.Add(time.Hour).Format("20060102T150405.000Z") },
		func(s *moderationRaidSeason) { s.StartTime = "" },
		func(s *moderationRaidSeason) { s.TotalAttacks = nil },
		func(s *moderationRaidSeason) { s.TotalAttacks = intPointer(14) },
		func(s *moderationRaidSeason) { s.Members = nil },
	} {
		s := season
		change(&s)
		if len(get(s, roster)) != 0 {
			t.Fatal("incomplete raid data treated as zero")
		}
	}
	// The API may omit members when the explicit aggregate is zero.
	season.Members = nil
	season.TotalAttacks = intPointer(0)
	if got := get(season, roster); len(got) != 2 {
		t.Fatal("confirmed empty raid lost historical roster", got)
	}
}

func TestRaidAutocheckWaitsForFinalResponseThenDeduplicatesPartialAttacks(t *testing.T) {
	a := adminTestApp(t)
	a.token = "test-token"
	now := time.Now().UTC()
	start := now.Add(-3 * 24 * time.Hour).Format("20060102T150405.000Z")
	end := now.Add(-time.Hour).Format("20060102T150405.000Z")
	state := "ongoing"
	a.clash = &http.Client{Transport: clashTransport(func(r *http.Request) (*http.Response, error) {
		body, status := `{"state":"notInWar"}`, 200
		switch {
		case strings.Contains(r.URL.Path, "capitalraidseasons"):
			body = `{"items":[{"state":"` + state + `","startTime":"` + start + `","endTime":"` + end + `","totalAttacks":3,"members":[{"tag":"#P0Y8LQ","name":"Parcial","attacks":3,"attackLimit":5}]}]}`
		case strings.HasSuffix(r.URL.Path, "/leaguegroup"):
			body, status = `{"reason":"notFound"}`, 404
		case strings.HasSuffix(r.URL.Path, "/clans/#P0Y8LQ"):
			body = `{"tag":"#P0Y8LQ","memberList":[{"tag":"#P0Y8LQ","name":"Parcial"}]}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	if count, _, err := a.collectWarnings(context.Background(), "P0Y8LQ"); err != nil || count != 0 {
		t.Fatal("ongoing raid was checked as final", count, err)
	}
	// Even an ended payload fetched before the deadline is not final evidence.
	state = "ended"
	endpoint := "/clans/%23P0Y8LQ/capitalraidseasons?limit=10"
	for key, entry := range a.cache {
		if strings.Contains(key, "capitalraidseasons") {
			endpoint = key
			entry.body = []byte(strings.Replace(string(entry.body), `"ongoing"`, `"ended"`, 1))
			entry.fetchedAt = now.Add(-2 * time.Hour)
			a.cache[key] = entry
		}
	}
	if count, _, err := a.collectWarnings(context.Background(), "P0Y8LQ"); err != nil || count != 0 {
		t.Fatal("pre-deadline cache treated as final", endpoint, count, err)
	}
	a.cache = nil
	if count, _, err := a.collectWarnings(context.Background(), "P0Y8LQ"); err != nil || count != 1 {
		t.Fatal("closed partial raid not recorded", count, err)
	}
	if count, _, err := a.collectWarnings(context.Background(), "P0Y8LQ"); err != nil || count != 0 {
		t.Fatal("repeat check duplicated a warning", count, err)
	}
	records, err := readModerationRecords(context.Background(), a.db, "P0Y8LQ")
	if err != nil || len(records) != 1 || !strings.Contains(records[0].Reason, "3/5") || records[0].Status != "pending" {
		t.Fatal(records, err)
	}
}
