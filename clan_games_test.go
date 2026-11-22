package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func gamesTestDocument(t *testing.T) (clanGamesDocument, time.Time, time.Time, time.Time, time.Time) {
	t.Helper()
	event, err := defaultClanGamesEvent("2026-11")
	if err != nil {
		t.Fatal(err)
	}
	start, end := clanGamesTimes(event)
	floor := time.Date(2026, 10, 28, 8, 10, 0, 0, time.UTC)
	cutoff := time.Date(2026, 12, 22, 8, 0, 0, 0, time.UTC)
	return clanGamesDocument{Event: event, Members: map[string]*clanGamesSnapshot{}, Problems: []string{}}, start, end, floor, cutoff
}

func gamesTestProfile(value int64) clanGamesProfile {
	var profile clanGamesProfile
	_ = json.Unmarshal([]byte(fmt.Sprintf(`{"tag":"#P0Y8LQ","name":"Jogador","townHallLevel":15,"clan":{"tag":"#QY8L0P"},"achievements":[{"name":"Games Champion","value":%d}]}`, value)), &profile)
	return profile
}

func TestClanGamesBaselineAndFinalSnapshot(t *testing.T) {
	doc, start, end, floor, cutoff := gamesTestDocument(t)
	roster := []clanGamesRosterMember{{Tag: "#P0Y8LQ", Name: "Jogador", Role: "member", TownHallLevel: 15}}
	applyClanGamesRoster(&doc, roster, start.Add(-time.Hour))
	if err := applyClanGamesProfile(&doc, "#P0Y8LQ", gamesTestProfile(100000), "QY8L0P", start.Add(-time.Hour), floor, cutoff); err != nil {
		t.Fatal(err)
	}
	if row := clanGamesMemberRow(doc.Event, doc.Members["#P0Y8LQ"], start.Add(-time.Minute)); row.Status != "upcoming" || row.Points != nil {
		t.Fatal(row)
	}
	if err := applyClanGamesProfile(&doc, "#P0Y8LQ", gamesTestProfile(101000), "QY8L0P", start.Add(time.Hour), floor, cutoff); err != nil {
		t.Fatal(err)
	}
	view := clanGamesView(doc, start.Add(time.Hour))
	if view.Items[0].Status != "contributing" || *view.Items[0].Points != 1000 || view.Summary.Points != 1000 || view.Summary.Participants != 1 || *doc.Members["#P0Y8LQ"].Baseline != 100000 {
		t.Fatal(view)
	}
	if err := applyClanGamesProfile(&doc, "#P0Y8LQ", gamesTestProfile(104000), "QY8L0P", end.Add(time.Minute), floor, cutoff); err != nil {
		t.Fatal(err)
	}
	view = clanGamesView(doc, end.Add(time.Minute))
	if view.Items[0].Status != "review" || view.Event.Finalized {
		t.Fatal("premature final result", view)
	}
	if err := applyClanGamesProfile(&doc, "#P0Y8LQ", gamesTestProfile(104000), "QY8L0P", end.Add(11*time.Minute), floor, cutoff); err != nil {
		t.Fatal(err)
	}
	view = clanGamesView(doc, end.Add(11*time.Minute))
	if view.Items[0].Status != "completed" || !view.Event.Finalized || view.Summary.Completed != 1 {
		t.Fatal(view)
	}
	applyClanGamesRoster(&doc, []clanGamesRosterMember{}, end.Add(time.Hour))
	view = clanGamesView(doc, end.Add(time.Hour))
	if view.Items[0].Current || view.Items[0].Status != "completed" || view.Summary.Points != 4000 {
		t.Fatal("post-event departure changed final result", view)
	}
	if err := applyClanGamesProfile(&doc, "#P0Y8LQ", gamesTestProfile(108000), "QY8L0P", cutoff.Add(time.Hour), floor, cutoff); err != nil {
		t.Fatal(err)
	}
	if *doc.Members["#P0Y8LQ"].Latest != 104000 {
		t.Fatal("next event changed frozen total")
	}
}

func TestClanGamesUnknownAndFailureStatesNeverBecomeConfirmedZero(t *testing.T) {
	cases := []string{"late start", "cached late arrival", "profile failure", "counter regression", "clan mismatch", "departure", "return", "stale", "above cap", "before first sample"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			doc, start, _, floor, cutoff := gamesTestDocument(t)
			roster := []clanGamesRosterMember{{Tag: "#P0Y8LQ", Name: "Jogador", Role: "member", TownHallLevel: 15}}
			at := start.Add(-time.Hour)
			if name == "late start" || name == "cached late arrival" {
				at = start.Add(time.Minute)
			}
			applyClanGamesRoster(&doc, roster, at)
			profileAt := at
			if name == "cached late arrival" {
				profileAt = start.Add(-time.Minute)
			}
			if err := applyClanGamesProfile(&doc, "#P0Y8LQ", gamesTestProfile(100000), "QY8L0P", profileAt, floor, cutoff); err != nil {
				t.Fatal(err)
			}
			now := start.Add(2 * time.Minute)
			if name != "before first sample" {
				if err := applyClanGamesProfile(&doc, "#P0Y8LQ", gamesTestProfile(100000), "QY8L0P", now, floor, cutoff); err != nil {
					t.Fatal(err)
				}
			}
			member := doc.Members["#P0Y8LQ"]
			switch name {
			case "profile failure":
				member.Failure = "Consulta indisponível."
			case "counter regression":
				if err := applyClanGamesProfile(&doc, member.Tag, gamesTestProfile(99000), "QY8L0P", now.Add(time.Minute), floor, cutoff); err != nil {
					t.Fatal(err)
				}
			case "clan mismatch":
				profile := gamesTestProfile(100000)
				profile.Clan.Tag = "#OTHER"
				if err := applyClanGamesProfile(&doc, member.Tag, profile, "QY8L0P", now.Add(time.Minute), floor, cutoff); err == nil {
					t.Fatal("mismatched clan accepted")
				}
			case "departure", "return":
				applyClanGamesRoster(&doc, []clanGamesRosterMember{}, now.Add(time.Minute))
				if name == "return" {
					applyClanGamesRoster(&doc, roster, now.Add(2*time.Minute))
				}
			case "stale":
				now = now.Add(time.Hour)
			case "above cap":
				if err := applyClanGamesProfile(&doc, member.Tag, gamesTestProfile(110000), "QY8L0P", now.Add(time.Minute), floor, cutoff); err != nil {
					t.Fatal(err)
				}
			}
			view := clanGamesView(doc, now)
			if view.Items[0].Status != "review" || !view.Items[0].Partial || view.Items[0].Reason == "" || view.Summary.Zero != 0 || view.Summary.Points != 0 || view.Summary.Participants != 0 {
				t.Fatal(view)
			}
		})
	}
}

func TestClanGamesEligibilityZeroAndRoleTargets(t *testing.T) {
	doc, start, _, floor, cutoff := gamesTestDocument(t)
	applyClanGamesRoster(&doc, []clanGamesRosterMember{{Tag: "#P0Y8LQ", Name: "Jogador", Role: "admin", TownHallLevel: 15}}, start.Add(-time.Minute))
	applyClanGamesProfile(&doc, "#P0Y8LQ", gamesTestProfile(100000), "QY8L0P", start.Add(-time.Minute), floor, cutoff)
	applyClanGamesProfile(&doc, "#P0Y8LQ", gamesTestProfile(100000), "QY8L0P", start.Add(time.Minute), floor, cutoff)
	doc.Event.ElderTarget = 2000
	view := clanGamesView(doc, start.Add(time.Minute))
	if view.Items[0].Status != "zero" || view.Items[0].Target != 2000 || view.Summary.Zero != 1 || view.Summary.Eligible != 1 {
		t.Fatal(view)
	}
	doc.Members["#P0Y8LQ"].TownHallLevel = 5
	view = clanGamesView(doc, start.Add(time.Minute))
	if view.Items[0].Status != "ineligible" || view.Items[0].Points != nil || view.Summary.Zero != 0 || view.Summary.Eligible != 0 {
		t.Fatal(view)
	}
}

func TestClanGamesRejectsUnboundedAndInvalidProfiles(t *testing.T) {
	for _, name := range []string{"before window", "next event", "missing value", "negative value", "wrong player"} {
		t.Run(name, func(t *testing.T) {
			doc, start, _, floor, cutoff := gamesTestDocument(t)
			applyClanGamesRoster(&doc, []clanGamesRosterMember{{Tag: "#P0Y8LQ", TownHallLevel: 15}}, start.Add(-time.Hour))
			profile := gamesTestProfile(100)
			at := start.Add(-time.Minute)
			switch name {
			case "before window":
				at = floor.Add(-time.Nanosecond)
			case "next event":
				at = cutoff
			case "missing value":
				profile.Achievements[0].Value = nil
			case "negative value":
				profile = gamesTestProfile(-1)
			case "wrong player":
				profile.Tag = "#OTHER"
			}
			if err := applyClanGamesProfile(&doc, "#P0Y8LQ", profile, "QY8L0P", at, floor, cutoff); err == nil || doc.Members["#P0Y8LQ"].Baseline != nil {
				t.Fatal("invalid baseline accepted", doc.Members["#P0Y8LQ"], err)
			}
		})
	}
}

func TestClanGamesPersistenceAndOptimisticUpdatesAcrossDatabases(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprint("postgres=", postgres), func(t *testing.T) {
			db := integrationDatabase(t, postgres)
			if err := initClanGames(db); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			doc, start, _, floor, cutoff := gamesTestDocument(t)
			applyClanGamesRoster(&doc, []clanGamesRosterMember{{Tag: "#P0Y8LQ", TownHallLevel: 15}}, start.Add(-time.Minute))
			applyClanGamesProfile(&doc, "#P0Y8LQ", gamesTestProfile(100000), "QY8L0P", start.Add(-time.Minute), floor, cutoff)
			if err := writeClanGamesDocument(ctx, db, "QY8L0P", &doc, 0); err != nil {
				t.Fatal(err)
			}
			loaded, revision, err := readClanGamesDocument(ctx, db, "QY8L0P", doc.Event.Season)
			if err != nil || revision != 1 || *loaded.Members["#P0Y8LQ"].Baseline != 100000 {
				t.Fatal(loaded, revision, err)
			}
			loaded.Event.MemberTarget = 2000
			if err = writeClanGamesDocument(ctx, db, "QY8L0P", &loaded, revision); err != nil {
				t.Fatal(err)
			}
			if err = writeClanGamesDocument(ctx, db, "QY8L0P", &doc, revision); !errors.Is(err, errClanGamesConflict) {
				t.Fatal("stale update accepted", err)
			}
			if err = initClanGames(db); err != nil {
				t.Fatal(err)
			}
			loaded, revision, err = readClanGamesDocument(ctx, db, "QY8L0P", doc.Event.Season)
			if err != nil || revision != 2 || loaded.Event.MemberTarget != 2000 || *loaded.Members["#P0Y8LQ"].Baseline != 100000 {
				t.Fatal("reinitialization lost history", loaded, revision, err)
			}
			other, revision, err := readClanGamesDocument(ctx, db, "OTHER", doc.Event.Season)
			if err != nil || revision != 0 || len(other.Members) != 0 {
				t.Fatal("cross-clan data", other, revision, err)
			}
		})
	}
}

func TestClanGamesCollectionPreservesFailureEvidence(t *testing.T) {
	db := integrationDatabase(t, false)
	if err := initClanGames(db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	doc, _, err := readClanGamesDocument(context.Background(), db, "QY8L0P", now.Format("2006-01"))
	if err != nil {
		t.Fatal(err)
	}
	doc.Event.StartTime, doc.Event.EndTime = now.Add(-time.Hour).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339)
	baseline := int64(100000)
	doc.Members["#P0Y8LQ"] = &clanGamesSnapshot{Tag: "#P0Y8LQ", Name: "Jogador", Role: "member", TownHallLevel: 15, Current: true, Baseline: &baseline, Latest: &baseline, BaselineAt: now.Add(-2 * time.Hour).Format(time.RFC3339Nano), UpdatedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)}
	if err = writeClanGamesDocument(context.Background(), db, "QY8L0P", &doc, 0); err != nil {
		t.Fatal(err)
	}
	a := &app{db: db, token: "test-token", clash: &http.Client{Transport: clashTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, "/v1/players/") {
			return clashResponse(503, `{"reason":"maintenance"}`), nil
		}
		return clashResponse(200, `{"tag":"#QY8L0P","memberList":[{"tag":"#P0Y8LQ","name":"Jogador","role":"member","townHallLevel":15}]}`), nil
	})}}
	if err = a.collectClanGames(context.Background(), "QY8L0P", doc.Event.Season, now); err != nil {
		t.Fatal(err)
	}
	response, err := a.clanGamesResponse(context.Background(), "QY8L0P", doc.Event.Season, now)
	if err != nil || len(response.Problems) == 0 || response.Items[0].Status != "review" || response.Items[0].Points == nil || *response.Items[0].Points != 0 || response.Summary.Zero != 0 {
		t.Fatal(response, err)
	}
	loaded, _, _ := readClanGamesDocument(context.Background(), db, "QY8L0P", doc.Event.Season)
	if *loaded.Members["#P0Y8LQ"].Baseline != 100000 {
		t.Fatal("failed profile overwrote baseline")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	loaded, revision, _ := readClanGamesDocument(context.Background(), db, "QY8L0P", doc.Event.Season)
	if err = a.saveCollectedClanGames(canceled, "QY8L0P", &loaded, revision); err != nil {
		t.Fatal("canceled sync lost evidence", err)
	}
	loaded, _, _ = readClanGamesDocument(context.Background(), db, "QY8L0P", doc.Event.Season)
	if len(loaded.Problems) < 2 {
		t.Fatal("interrupted collection not disclosed", loaded.Problems)
	}
}

func TestClanGamesHTTPSettingsAndSelectedSeason(t *testing.T) {
	db := integrationDatabase(t, false)
	if err := initClanGames(db); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	a := &app{db: db, clanTags: map[string]string{"1": "QY8L0P"}, token: "test-token", clash: &http.Client{Transport: clashTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return clashResponse(200, `{"tag":"#QY8L0P","memberList":[]}`), nil
	})}}
	nextSeason := time.Now().UTC().AddDate(0, 2, 0).Format("2006-01")
	w := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/clan-games/sync?clan=1&season="+nextSeason, strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	a.syncClanGames(w, request)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"season":"`+nextSeason+`"`) || calls.Load() != 0 {
		t.Fatal("selected season ignored or early baseline fetched", w.Code, w.Body.String(), calls.Load())
	}
	event, _ := defaultClanGamesEvent(nextSeason)
	event.PlayerCap, event.ElderTarget, event.CoLeaderTarget, event.LeaderTarget = 10000, 7000, 10000, 10000
	body, _ := json.Marshal(event)
	r := httptest.NewRequest("PUT", "/api/clan-games/settings?clan=1", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	a.saveClanGamesSettings(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	event.MemberTarget = 12000
	body, _ = json.Marshal(event)
	r = httptest.NewRequest("PUT", "/api/clan-games/settings?clan=1", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	a.saveClanGamesSettings(w, r)
	if w.Code != 400 {
		t.Fatal("impossible role target accepted", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	a.clanGames(w, httptest.NewRequest("GET", "/api/clan-games?clan=1&season="+nextSeason, nil))
	if w.Code != 200 || calls.Load() != 0 || !strings.Contains(w.Body.String(), `"elderTarget":7000`) {
		t.Fatal(w.Code, w.Body.String())
	}
	currentSeason := time.Now().UTC().Format("2006-01")
	doc, _, _ := readClanGamesDocument(context.Background(), db, "QY8L0P", currentSeason)
	doc.Event.StartTime = time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	doc.Event.EndTime = time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	baseline, latest := int64(100000), int64(100400)
	doc.Members["#P0Y8LQ"] = &clanGamesSnapshot{Tag: "#P0Y8LQ", Name: "Jogador", Role: "member", TownHallLevel: 15, Current: true, Baseline: &baseline, Latest: &latest, BaselineAt: time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := writeClanGamesDocument(context.Background(), db, "QY8L0P", &doc, 0); err != nil {
		t.Fatal(err)
	}
	start, end := clanGamesTimes(doc.Event)
	doc.Event.StartTime = start.Format("2006-01-02T15:04:05.000Z")
	doc.Event.EndTime = end.Format("2006-01-02T15:04:05.000Z")
	doc.Event.MemberTarget, doc.Event.PlayerCap = 3500, 10000
	body, _ = json.Marshal(doc.Event)
	r = httptest.NewRequest("PUT", "/api/clan-games/settings?clan=1", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	a.saveClanGamesSettings(w, r)
	if w.Code != 200 {
		t.Fatal("equivalent timestamps blocked target-only edit", w.Code, w.Body.String())
	}
	doc, _, _ = readClanGamesDocument(context.Background(), db, "QY8L0P", currentSeason)
	if doc.Event.MemberTarget != 3500 || doc.Event.PlayerCap != 10000 || *doc.Members["#P0Y8LQ"].Baseline != baseline || *doc.Members["#P0Y8LQ"].Latest != latest || doc.Event.StartTime != start.Format(time.RFC3339) || doc.Event.EndTime != end.Format(time.RFC3339) {
		t.Fatal("target-only edit changed evidence or failed to normalize dates", doc)
	}
	doc.Event.StartTime = time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339)
	body, _ = json.Marshal(doc.Event)
	r = httptest.NewRequest("PUT", "/api/clan-games/settings?clan=1", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	a.saveClanGamesSettings(w, r)
	if w.Code != 400 {
		t.Fatal("active schedule edit accepted", w.Code, w.Body.String())
	}
}

func TestClanGamesSeasonBoundariesAndWorkerAvoidHistoricalBackfill(t *testing.T) {
	db := integrationDatabase(t, false)
	if err := initClanGames(db); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	a := &app{db: db, clanTags: map[string]string{"1": "QY8L0P"}, token: "test-token", clash: &http.Client{Transport: clashTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return clashResponse(200, `{"tag":"#QY8L0P","memberList":[]}`), nil
	})}}
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	a.trackClanGamesOnce(now)
	_, previousRevision, err := readClanGamesDocument(context.Background(), db, "QY8L0P", "2026-10")
	if err != nil || previousRevision != 0 {
		t.Fatal("worker invented previous event", previousRevision, err)
	}
	count := calls.Load()
	if err = a.collectClanGames(context.Background(), "QY8L0P", "2026-12", time.Date(2026, 11, 28, 8, 5, 0, 0, time.UTC)); err != nil || calls.Load() != count {
		t.Fatal("next baseline captured before prior event settled", err, calls.Load())
	}
	if err = a.collectClanGames(context.Background(), "QY8L0P", "2026-10", time.Date(2026, 11, 22, 8, 0, 0, 0, time.UTC)); err != nil || calls.Load() != count {
		t.Fatal("old season absorbed next event", err, calls.Load())
	}
	for _, season := range []string{"2026-1", "2026-13", "../2026-11", "2019-12"} {
		if _, err = clanGamesSeason(season); err == nil {
			t.Fatal("invalid season accepted", season)
		}
	}
}
