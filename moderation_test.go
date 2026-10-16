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

func TestAutomaticWarWarningsOnlyZeroAttacksInConfirmedLineup(t *testing.T) {
	now := time.Now().UTC()
	since := now.Add(-7 * 24 * time.Hour)
	war := moderationWar{State: "warEnded", EndTime: now.Add(-time.Hour).Format("20060102T150405.000Z"), AttacksPerMember: 2, Clan: moderationSide{Tag: "#P0Y8LQ", Members: []moderationMember{{Tag: "#P0Y8LQ", Name: "Zero"}, {Tag: "#QY8L0P", Name: "Parcial", Attacks: []json.RawMessage{json.RawMessage(`{}`)}}}}, Opponent: moderationSide{Tag: "#QQQQ"}}
	body, _ := json.Marshal(war)
	records := warWarnings(body, "P0Y8LQ", "war", war.EndTime, since, now)
	if len(records) != 1 || records[0].PlayerName != "Zero" || records[0].Status != "active" {
		t.Fatalf("incorrect warnings: %+v", records)
	}
	for _, state := range []string{"preparation", "inWar", "notInWar"} {
		war.State = state
		body, _ = json.Marshal(war)
		if len(warWarnings(body, "P0Y8LQ", "war", war.EndTime, since, now)) != 0 {
			t.Fatal("warned unfinished event")
		}
	}
	war.State = "warEnded"
	war.AttacksPerMember = 1
	war.Clan, war.Opponent = war.Opponent, war.Clan
	body, _ = json.Marshal(war)
	if got := warWarnings(body, "P0Y8LQ", "cwl", "QY8L0P", since, now); len(got) != 1 || got[0].Source != "cwl" {
		t.Fatal("CWL own opponent-side lineup not respected", got)
	}
	if len(warWarnings(body, "PPPP", "cwl", "QY8L0P", since, now)) != 0 {
		t.Fatal("unrelated clan warned")
	}
	if len(warWarnings(body, "P0Y8LQ", "cwl", "QY8L0P", now, now)) != 0 {
		t.Fatal("historical outside window warned")
	}
}
func TestRaidZeroNeedsRecordedRosterAndReview(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(-3 * 24 * time.Hour).Format("20060102T150405.000Z")
	end := now.Add(-time.Hour).Format("20060102T150405.000Z")
	body, _ := json.Marshal(map[string]any{"items": []any{map[string]any{"startTime": start, "endTime": end, "members": []any{map[string]any{"tag": "#QY8L0P", "name": "Parcial", "attacks": 1, "attackLimit": 5, "bonusAttackLimit": 0}}}}})
	if len(raidWarnings(body, nil, now.Add(-7*24*time.Hour), now)) != 0 {
		t.Fatal("invented warning without historical roster")
	}
	records := raidWarnings(body, map[string][]raidRosterMember{start: {{Tag: "#P0Y8LQ", Name: "Zero"}, {Tag: "#QY8L0P", Name: "Parcial"}}}, now.Add(-7*24*time.Hour), now)
	if len(records) != 1 || records[0].PlayerName != "Zero" || records[0].Status != "pending" {
		t.Fatalf("raid warning incorrectly counted: %+v", records)
	}
}
func TestModerationDeduplicatesAndKeepsDismissalsAcrossSyncs(t *testing.T) {
	a := adminTestApp(t)
	now := time.Now().UTC()
	end := now.Add(-time.Hour).Format("20060102T150405.000Z")
	a.token = "server-test-token"
	a.clash = &http.Client{Transport: clashTransport(func(r *http.Request) (*http.Response, error) {
		body, status := `{"items":[]}`, 200
		if strings.HasSuffix(r.URL.Path, "/currentwar") {
			body = `{"state":"warEnded","endTime":"` + end + `","attacksPerMember":2,"clan":{"tag":"#P0Y8LQ","members":[{"tag":"#P0Y8LQ","name":"Zero"}]}}`
		}
		if strings.HasSuffix(r.URL.Path, "/leaguegroup") {
			status = 404
			body = `{"reason":"notFound"}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	count, _, err := a.collectWarnings(context.Background(), "P0Y8LQ")
	if err != nil || count != 1 {
		t.Fatal(count, err)
	}
	count, _, err = a.collectWarnings(context.Background(), "P0Y8LQ")
	if err != nil || count != 0 {
		t.Fatal("duplicate warnings", count, err)
	}
	handler := adminTestRouter(a)
	list := adminTestRequest(handler, "GET", "https://dashboard.imperiobarbaro.com.br/api/moderation?clan=1", nil, true)
	var data struct {
		Items []moderationRecord `json:"items"`
	}
	json.Unmarshal(list.Body.Bytes(), &data)
	if len(data.Items) != 1 {
		t.Fatal(list.Body.String())
	}
	record := data.Items[0]
	input := map[string]any{"revision": record.Revision, "playerTag": record.PlayerTag, "playerName": record.PlayerName, "kind": record.Kind, "status": "dismissed", "reason": record.Reason, "action": "", "notes": "Removido após conferência."}
	res := adminTestRequest(handler, "PUT", "https://dashboard.imperiobarbaro.com.br/api/moderation/"+record.ID+"?clan=1", input, true)
	if res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	if res = adminTestRequest(handler, "PUT", "https://dashboard.imperiobarbaro.com.br/api/moderation/"+record.ID+"?clan=1", input, true); res.Code != 409 {
		t.Fatal("lost-update accepted", res.Code)
	}
	if res = adminTestRequest(handler, "PUT", "https://dashboard.imperiobarbaro.com.br/api/moderation/"+record.ID+"?clan=2", input, true); res.Code != 404 {
		t.Fatal("cross-clan edit accepted", res.Code)
	}
	if err = initModeration(a.db); err != nil {
		t.Fatal(err)
	}
	if count, _, err = a.collectWarnings(context.Background(), "P0Y8LQ"); err != nil || count != 0 {
		t.Fatal("dismissed warning recreated", count, err)
	}
	list = adminTestRequest(handler, "GET", "https://dashboard.imperiobarbaro.com.br/api/moderation?clan=1", nil, true)
	json.Unmarshal(list.Body.Bytes(), &data)
	if data.Items[0].Status != "dismissed" {
		t.Fatal("dismissal lost")
	}
}
func TestManualWarningsPunishmentsAndAutomationSettings(t *testing.T) {
	a := adminTestApp(t)
	handler := adminTestRouter(a)
	record := map[string]any{"playerTag": "#P0Y8LQ", "playerName": "Jogador", "kind": "punishment", "status": "active", "reason": "Dois avisos confirmados.", "action": "", "notes": ""}
	if res := adminTestRequest(handler, "POST", "https://dashboard.imperiobarbaro.com.br/api/moderation?clan=1", record, true); res.Code != 400 {
		t.Fatal("punishment without decision accepted")
	}
	record["action"] = "Fora das próximas duas guerras."
	if res := adminTestRequest(handler, "POST", "https://dashboard.imperiobarbaro.com.br/api/moderation?clan=1", record, true); res.Code != 201 {
		t.Fatal(res.Code, res.Body.String())
	}
	record["kind"] = "warning"
	record["action"] = ""
	if res := adminTestRequest(handler, "POST", "https://dashboard.imperiobarbaro.com.br/api/moderation?clan=2", record, true); res.Code != 201 {
		t.Fatal(res.Code, res.Body.String())
	}
	settings, err := readModerationSettings(context.Background(), a.db, "P0Y8LQ")
	if err != nil || settings.Automatic {
		t.Fatal("automation enabled without selection")
	}
	input := map[string]any{"revision": settings.Revision, "automatic": true}
	if res := adminTestRequest(handler, "PUT", "https://dashboard.imperiobarbaro.com.br/api/moderation-settings?clan=1", input, true); res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	if res := adminTestRequest(handler, "PUT", "https://dashboard.imperiobarbaro.com.br/api/moderation-settings?clan=1", input, true); res.Code != 409 {
		t.Fatal("stale configuration accepted")
	}
	settings, err = readModerationSettings(context.Background(), a.db, "P0Y8LQ")
	if err != nil || !settings.Automatic {
		t.Fatal("automation config lost")
	}
	other, _ := readModerationSettings(context.Background(), a.db, "QY8L0P")
	if other.Automatic {
		t.Fatal("automation leaked between clans")
	}
}
