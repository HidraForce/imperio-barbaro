package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMemberTrackingAcrossDatabases(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprint("postgres=", postgres), func(t *testing.T) {
			db := integrationDatabase(t, postgres)
			if err := initMembers(db); err != nil {
				t.Fatal(err)
			}
			a := &app{db: db}
			ctx := context.Background()
			now := time.Now().UTC()
			baseline := now.Add(-3 * 24 * time.Hour)
			roster := []byte(`{"tag":"#P0Y8LQ","memberList":[{"tag":"#P0Y8LQ","name":"Existing"}]}`)
			observe := func(body []byte, at time.Time) {
				t.Helper()
				if err := a.observeMembers(ctx, "P0Y8LQ", body, at); err != nil {
					t.Fatal(err)
				}
			}
			observe(roster, baseline)
			members, err := a.memberObservations(ctx, "P0Y8LQ", now)
			if err != nil || members["#P0Y8LQ"].NewMember || members["#P0Y8LQ"].DetectedJoin {
				t.Fatal("baseline treated as new", members, err)
			}
			observe([]byte(`{"tag":"#P0Y8LQ","memberList":[]}`), now.Add(-2*time.Hour))
			observe(roster, now.Add(-time.Hour))
			members, err = a.memberObservations(ctx, "P0Y8LQ", now)
			m := members["#P0Y8LQ"]
			if err != nil || !m.NewMember || !m.DetectedJoin || m.JoinAfter != now.Add(-2*time.Hour).Format(time.RFC3339Nano) || m.FirstSeen != now.Add(-time.Hour).Format(time.RFC3339Nano) {
				t.Fatal("rejoin not tracked", m, err)
			}
			observe(roster, baseline)
			for _, bad := range []string{`{"tag":"#P0Y8LQ"}`, `{"tag":"#OTHER","memberList":[]}`, `{"tag":"#P0Y8LQ","memberList":[{"tag":"#P0Y8LQ"},{"tag":"#P0Y8LQ"}]}`} {
				if a.observeMembers(ctx, "P0Y8LQ", []byte(bad), now) == nil {
					t.Fatal("invalid roster changed history")
				}
			}
			if err := initMembers(db); err != nil {
				t.Fatal(err)
			}
			members, _ = a.memberObservations(ctx, "P0Y8LQ", now.Add(8*24*time.Hour))
			if members["#P0Y8LQ"].FirstSeen != m.FirstSeen || members["#P0Y8LQ"].NewMember {
				t.Fatal("history lost or new marker never expires")
			}
			other, err := a.memberObservations(ctx, "QY8L0P", now)
			if err != nil || len(other) != 0 {
				t.Fatal("cross-clan history", other, err)
			}
		})
	}
}

func TestLateJoinWarningEligibility(t *testing.T) {
	start, end := "20261010T070000.000Z", "20261012T070000.000Z"
	for _, source := range []string{"war", "cwl", "raids"} {
		record := moderationRecord{PlayerTag: "#P0Y8LQ", Kind: "warning", Status: "active", Source: source, EventStart: start, EventEnd: end}
		cases := []struct {
			name, first, after, status string
			detected, available        bool
			count                      int
		}{
			{"joined after event", "2026-10-13T07:00:00Z", "2026-10-12T07:00:00Z", "", true, true, 0},
			{"uncertain late arrival", "2026-10-13T07:00:00Z", "2026-10-11T07:00:00Z", "pending", true, true, 1},
			{"joined during event", "2026-10-11T07:00:00Z", "2026-10-10T08:00:00Z", "pending", true, true, 1},
			{"present before event", "2026-10-09T07:00:00Z", "2026-10-08T07:00:00Z", "active", true, true, 1},
			{"initial baseline not a join date", "2026-10-13T07:00:00Z", "", "active", false, true, 1},
			{"membership unavailable", "", "", "pending", false, false, 1},
		}
		for _, tc := range cases {
			t.Run(source+"/"+tc.name, func(t *testing.T) {
				members := map[string]memberObservation{"#P0Y8LQ": {FirstSeen: tc.first, JoinAfter: tc.after, DetectedJoin: tc.detected}}
				got := membershipWarnings([]moderationRecord{record}, members, tc.available)
				if len(got) != tc.count || (len(got) > 0 && (got[0].Status != tc.status || got[0].Kind != "warning")) {
					t.Fatal(got)
				}
				if len(got) > 0 && got[0].Status == "pending" && got[0].Notes == "" {
					t.Fatal("review reason missing")
				}
			})
		}
	}
}

func TestClanIncludesObservedEntryWithoutInventingBaselineDate(t *testing.T) {
	a := adminTestApp(t)
	if err := a.observeMembers(context.Background(), "P0Y8LQ", []byte(`{"tag":"#P0Y8LQ","memberList":[{"tag":"#P0Y8LQ","name":"Old"}]}`), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	a.token = "test-token"
	a.clash = &http.Client{Transport: clashTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"tag":"#P0Y8LQ","name":"Clan","memberList":[{"tag":"#P0Y8LQ","name":"Old","trophies":100},{"tag":"#QY8L0P","name":"New"}]}`))}, nil
	})}
	w := httptest.NewRecorder()
	a.clan(w, httptest.NewRequest("GET", "/api/clan?clan=1", nil))
	var result struct {
		MemberList []struct {
			Tag        string            `json:"tag"`
			Trophies   int               `json:"trophies"`
			Membership memberObservation `json:"membership"`
		} `json:"memberList"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.MemberList) != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
	if result.MemberList[0].Membership.NewMember || result.MemberList[0].Trophies != 100 || !result.MemberList[1].Membership.NewMember {
		t.Fatal(result)
	}
}

func TestCollectWarningsProtectsArrivalsAfterHistoricalEvents(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		t.Run(fmt.Sprint("confirmed=", confirmed), func(t *testing.T) {
			a := adminTestApp(t)
			now := time.Now().UTC()
			baseline := now.Add(-72 * time.Hour)
			if confirmed {
				baseline = now.Add(-12 * time.Hour)
			}
			if err := a.observeMembers(context.Background(), "P0Y8LQ", []byte(`{"tag":"#P0Y8LQ","memberList":[]}`), baseline); err != nil {
				t.Fatal(err)
			}
			a.token = "test-token"
			a.clash = &http.Client{Transport: clashTransport(func(r *http.Request) (*http.Response, error) {
				body, status := `{"items":[]}`, 200
				switch {
				case strings.HasSuffix(r.URL.Path, "/currentwar"):
					body = `{"state":"warEnded","startTime":"` + now.Add(-48*time.Hour).Format("20060102T150405.000Z") + `","endTime":"` + now.Add(-24*time.Hour).Format("20060102T150405.000Z") + `","clan":{"tag":"#P0Y8LQ","members":[{"tag":"#QY8L0P","name":"Novo"}]}}`
				case strings.HasSuffix(r.URL.Path, "/leaguegroup"):
					body, status = `{"reason":"notFound"}`, 404
				case strings.HasSuffix(r.URL.Path, "/clans/#P0Y8LQ"):
					body = `{"tag":"#P0Y8LQ","memberList":[{"tag":"#QY8L0P","name":"Novo"}]}`
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			count, problems, err := a.collectWarnings(context.Background(), "P0Y8LQ")
			expected := 1
			if confirmed {
				expected = 0
			}
			if err != nil || count != expected || len(problems) != 0 {
				t.Fatal(count, problems, err)
			}
			var active int
			if err := a.db.QueryRow(`SELECT COUNT(*) FROM moderation_records WHERE clan_tag=? AND payload LIKE ?`, "P0Y8LQ", `%"status":"active"%`).Scan(&active); err != nil || active != 0 {
				t.Fatal("new arrival received active warning", active, err)
			}
		})
	}
}

func TestRaidRosterPreservesEventMembershipAcrossDatabases(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprint("postgres=", postgres), func(t *testing.T) {
			db := integrationDatabase(t, postgres)
			now := time.Now().UTC()
			start, end := now.Add(-time.Hour).Format("20060102T150405.000Z"), now.Add(time.Hour).Format("20060102T150405.000Z")
			season := `{"startTime":"` + start + `","endTime":"` + end + `","members":[]}`
			_, err := db.Exec(`INSERT INTO raid_reports(clan_tag,start_time,season,roster,roster_captured_at,roster_during_raid,synced_at) VALUES(?,?,?,?,?,0,?)`, "P0Y8LQ", start, season, `[{"tag":"#QY8L0P","name":"Before event"}]`, now.Add(-2*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339))
			if err != nil {
				t.Fatal(err)
			}
			name := "During event"
			a := &app{db: db, token: "test-token", clanTags: map[string]string{"1": "P0Y8LQ"}}
			a.clash = &http.Client{Transport: clashTransport(func(r *http.Request) (*http.Response, error) {
				body := `{"tag":"#P0Y8LQ","memberList":[{"tag":"#P0Y8LQ","name":"` + name + `"}]}`
				if strings.Contains(r.URL.Path, "capitalraidseasons") {
					body = `{"items":[` + season + `]}`
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			a.captureModerationRaidRoster(context.Background(), "P0Y8LQ", []byte(`{"items":[`+season+`]}`))
			records, err := a.savedRaids(context.Background(), "P0Y8LQ")
			if err != nil || len(records) != 1 || !records[0].RosterDuringRaid || records[0].Roster[0].Name != "During event" {
				t.Fatal("event capture did not replace baseline", records, err)
			}
			name = "Later roster"
			a.cache = nil
			if postgres {
				if _, err = db.Exec(`DELETE FROM crm_cache`); err != nil {
					t.Fatal(err)
				}
			}
			w := httptest.NewRecorder()
			a.raids(w, httptest.NewRequest("GET", "/api/raids?clan=1", nil))
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			records, err = a.savedRaids(context.Background(), "P0Y8LQ")
			if err != nil || records[0].Roster[0].Name != "During event" {
				t.Fatal("later roster overwrote event evidence", records, err)
			}
		})
	}
}
