package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func policyWarning(sector string, issued time.Time) moderationRecord {
	return moderationRecord{PlayerTag: "#P0Y8LQ", PlayerName: "Jogador", Kind: "warning", Sector: sector, Status: "active", Reason: "Aviso confirmado.", Source: "manual", CreatedAt: issued.Format(time.RFC3339Nano)}
}

func TestModerationDegrees(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		sectors []string
		degree  int
	}{
		{"first warning", []string{"wars"}, 1},
		{"different sectors add up", []string{"wars", "capital"}, 2},
		{"third warning", []string{"wars", "capital", "games"}, 3},
		{"four in two sectors is not expulsion", []string{"wars", "wars", "capital", "capital"}, 3},
		{"four in three sectors is not expulsion", []string{"wars", "wars", "capital", "games"}, 3},
		{"four in one sector", []string{"wars", "wars", "wars", "wars"}, 4},
		{"all four sectors", []string{"wars", "capital", "games", "replays"}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var records []moderationRecord
			for _, sector := range tc.sectors {
				records = append(records, policyWarning(sector, now.Add(-time.Hour)))
			}
			if degree := moderationDegree(evaluateModeration(records, now), "P0Y8LQ", now); degree != tc.degree {
				t.Fatalf("degree = %d, want %d", degree, tc.degree)
			}
		})
	}
}

func TestWarningExpiryResetsAcrossSectorsWithoutRevivingExpiredCycles(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	records := []moderationRecord{
		policyWarning("games", now.Add(-90*24*time.Hour)),
		policyWarning("wars", now.Add(-59*24*time.Hour)),
		policyWarning("capital", now.Add(-30*24*time.Hour)),
		policyWarning("replays", now.Add(-24*time.Hour)),
	}
	// Deliberately unordered, like a history returned newest first.
	records[0], records[3] = records[3], records[0]
	result := evaluateModeration(records, now)
	for i, record := range result {
		want := now.Add(29 * 24 * time.Hour)
		if i == 3 {
			want = now.Add(-60 * 24 * time.Hour)
		}
		if record.ExpiresAt != want.Format(time.RFC3339Nano) {
			t.Fatalf("wrong expiry for %s: %s", record.Sector, record.ExpiresAt)
		}
	}
	if degree := moderationDegree(result, "#P0Y8LQ", now); degree != 3 {
		t.Fatalf("expired cycle was revived: %d", degree)
	}
	if records[0].ExpiresAt != "" {
		t.Fatal("evaluation mutated stored history")
	}
	if moderationDegree(evaluateModeration(records, now.Add(29*24*time.Hour)), "#P0Y8LQ", now.Add(29*24*time.Hour)) != 0 {
		t.Fatal("warnings must expire at the exact boundary")
	}
	boundary := []moderationRecord{policyWarning("wars", now.Add(-30*24*time.Hour)), policyWarning("games", now)}
	if degree := moderationDegree(evaluateModeration(boundary, now), "#P0Y8LQ", now); degree != 1 {
		t.Fatal("new warning revived a cycle at the boundary", degree)
	}
}

func TestOnlyConfirmedClassifiedWarningsExtendExpiry(t *testing.T) {
	now := time.Now().UTC()
	records := []moderationRecord{policyWarning("wars", now.Add(-31*24*time.Hour))}
	for _, status := range []string{"pending", "justified", "resolved", "dismissed"} {
		r := policyWarning("games", now.Add(-time.Hour))
		r.Status = status
		records = append(records, r)
	}
	legacy := policyWarning("", now.Add(-time.Hour))
	other := policyWarning("games", now.Add(-time.Hour))
	other.PlayerTag = "#QY8L0P"
	punishment := policyWarning("wars", now.Add(-time.Hour))
	punishment.Kind = "punishment"
	records = append(records, legacy, other, punishment)
	if degree := moderationDegree(evaluateModeration(records, now), "#P0Y8LQ", now); degree != 0 {
		t.Fatalf("unconfirmed records extended expiry: %d", degree)
	}
	for source, sector := range map[string]string{"war": "wars", "cwl": "wars", "raids": "capital"} {
		r := legacy
		r.Source = source
		result := evaluateModeration([]moderationRecord{r}, now)
		if result[0].Sector != sector || !activeModerationWarning(result[0], now) {
			t.Fatal("automatic sector not inferred", result)
		}
	}
}

func TestModerationAPIEnforcesDegreesAndClanIsolation(t *testing.T) {
	a := adminTestApp(t)
	handler := adminTestRouter(a)
	endpoint := "https://dashboard.imperiobarbaro.com.br/api/moderation?clan=1"
	punishment := map[string]any{"playerTag": "#P0Y8LQ", "playerName": "Jogador", "kind": "punishment", "sanction": "two_wars", "status": "active", "reason": "Reincidência."}
	post := func(input any, want int) {
		t.Helper()
		res := adminTestRequest(handler, "POST", endpoint, input, true)
		if res.Code != want {
			t.Fatalf("status %d, want %d: %s", res.Code, want, res.Body.String())
		}
	}
	post(punishment, 400)
	warning := map[string]any{"playerTag": "#P0Y8LQ", "playerName": "Jogador", "kind": "warning", "sector": "wrong", "status": "active", "reason": "Aviso."}
	post(warning, 400)
	warning["sector"] = "wars"
	post(warning, 201)
	post(punishment, 400)
	warning["sector"] = "capital"
	post(warning, 201)
	post(punishment, 201)
	if res := adminTestRequest(handler, "POST", "https://dashboard.imperiobarbaro.com.br/api/moderation?clan=2", punishment, true); res.Code != 400 {
		t.Fatal("warnings leaked between clans")
	}
	punishment["sanction"] = "expulsion"
	post(punishment, 400)
	warning["sector"] = "games"
	post(warning, 201)
	post(punishment, 400)
	warning["sector"] = "replays"
	post(warning, 201)
	post(punishment, 201)
	punishment["sanction"] = "demotion"
	post(punishment, 400)
	res := adminTestRequest(handler, "GET", endpoint, nil, true)
	var data struct {
		Items []moderationRecord `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	for _, r := range data.Items {
		if r.Kind == "warning" && r.ExpiresAt == "" {
			t.Fatal("expiry missing from API")
		}
		if r.Sanction == "expulsion" && r.Action != moderationSanctions["expulsion"] {
			t.Fatal("wrong sanction action")
		}
	}
}

func TestModerationAPIExpiredEvidenceAndPendingActivation(t *testing.T) {
	a := adminTestApp(t)
	handler := adminTestRouter(a)
	endpoint := "https://dashboard.imperiobarbaro.com.br/api/moderation"
	for _, sector := range []string{"wars", "capital"} {
		record := policyWarning(sector, time.Now().UTC())
		if _, err := insertModeration(context.Background(), a.db, "P0Y8LQ", &record); err != nil {
			t.Fatal(err)
		}
		record.CreatedAt = time.Now().UTC().Add(-31 * 24 * time.Hour).Format(time.RFC3339Nano)
		record.IssuedAt = record.CreatedAt
		payload, _ := json.Marshal(record)
		if _, err := a.db.Exec(`UPDATE moderation_records SET payload=? WHERE id=?`, string(payload), record.ID); err != nil {
			t.Fatal(err)
		}
	}
	punishment := map[string]any{"playerTag": "#P0Y8LQ", "playerName": "Jogador", "kind": "punishment", "sanction": "two_wars", "status": "active", "reason": "Reincidência."}
	if res := adminTestRequest(handler, "POST", endpoint+"?clan=1", punishment, true); res.Code != 400 {
		t.Fatal("expired evidence authorized a punishment", res.Code, res.Body.String())
	}
	punishment["status"] = "pending"
	res := adminTestRequest(handler, "POST", endpoint+"?clan=1", punishment, true)
	if res.Code != 201 {
		t.Fatal(res.Code, res.Body.String())
	}
	var saved moderationRecord
	if err := json.Unmarshal(res.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	punishment["status"], punishment["revision"] = "active", saved.Revision
	if res := adminTestRequest(handler, "PUT", endpoint+"/"+saved.ID+"?clan=1", punishment, true); res.Code != 400 {
		t.Fatal("pending activation bypassed evidence checks", res.Code, res.Body.String())
	}
	for _, sector := range []string{"games", "replays"} {
		warning := map[string]any{"playerTag": "#P0Y8LQ", "playerName": "Jogador", "kind": "warning", "sector": sector, "status": "active", "reason": "Novo aviso."}
		if res := adminTestRequest(handler, "POST", endpoint+"?clan=1", warning, true); res.Code != 201 {
			t.Fatal(res.Code, res.Body.String())
		}
	}
	if res := adminTestRequest(handler, "PUT", endpoint+"/"+saved.ID+"?clan=1", punishment, true); res.Code != 200 {
		t.Fatal("confirmed new warnings did not allow activation", res.Code, res.Body.String())
	}
	res = adminTestRequest(handler, "GET", endpoint+"?clan=1", nil, true)
	var data struct {
		Items []moderationRecord `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if degree := moderationDegree(data.Items, "#P0Y8LQ", time.Now().UTC()); degree != 2 {
		t.Fatal("old warnings revived", degree)
	}
	if len(data.Items) != 5 {
		t.Fatal("expired history was lost", len(data.Items))
	}
}

func TestWarningReviewStartsClockButNotesDoNotRenewIt(t *testing.T) {
	a := adminTestApp(t)
	handler := adminTestRouter(a)
	record := policyWarning("capital", time.Now().UTC())
	record.Status = "pending"
	if _, err := insertModeration(context.Background(), a.db, "P0Y8LQ", &record); err != nil {
		t.Fatal(err)
	}
	record.CreatedAt = time.Now().UTC().Add(-40 * 24 * time.Hour).Format(time.RFC3339Nano)
	payload, _ := json.Marshal(record)
	if _, err := a.db.Exec(`UPDATE moderation_records SET payload=? WHERE id=?`, string(payload), record.ID); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"revision": record.Revision, "playerTag": record.PlayerTag, "playerName": record.PlayerName, "kind": "warning", "sector": "capital", "status": "active", "reason": record.Reason}
	endpoint := "https://dashboard.imperiobarbaro.com.br/api/moderation/" + record.ID + "?clan=1"
	res := adminTestRequest(handler, "PUT", endpoint, input, true)
	if res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	var confirmed moderationRecord
	if err := json.Unmarshal(res.Body.Bytes(), &confirmed); err != nil {
		t.Fatal(err)
	}
	if confirmed.IssuedAt == "" || confirmed.IssuedAt == confirmed.CreatedAt {
		t.Fatal("review did not start the warning clock")
	}
	if degree := moderationDegree(evaluateModeration([]moderationRecord{confirmed}, time.Now().UTC()), record.PlayerTag, time.Now().UTC()); degree != 1 {
		t.Fatal("reviewed warning immediately expired")
	}
	input["revision"], input["notes"] = confirmed.Revision, "Conferido pela liderança."
	res = adminTestRequest(handler, "PUT", endpoint, input, true)
	if res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	var edited moderationRecord
	if err := json.Unmarshal(res.Body.Bytes(), &edited); err != nil {
		t.Fatal(err)
	}
	if edited.IssuedAt != confirmed.IssuedAt {
		t.Fatal("editing notes renewed the warning clock")
	}
}
