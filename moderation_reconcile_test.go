package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestFinalCountsCorrectOnlyUnreviewedAutomaticOccurrences(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprint("postgres=", postgres), func(t *testing.T) {
			db := integrationDatabase(t, postgres)
			if err := initModeration(db); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			add := func(source, key, player, status string) moderationRecord {
				r := moderationRecord{PlayerTag: player, PlayerName: player, Kind: "warning", Sector: automaticSector(source), Source: source, EventKey: key, Reason: "Contagem antiga: zero ataques.", Status: status}
				if _, err := insertModeration(ctx, db, "P0Y8LQ", &r); err != nil {
					t.Fatal(err)
				}
				return r
			}
			raid := add("raids", "raids:start:PYQ", "#PYQ", "pending")
			war := add("war", "war:end:QPC", "#QPC", "active")
			cwl := add("cwl", "cwl:round:LGC", "#LGC", "active")
			reviewed := add("raids", "raids:start:PPPP", "#PPPP", "active")
			editable := add("raids", "raids:second:CCCC", "#CCCC", "pending")
			removed := add("raids", "raids:start:LLLL", "#LLLL", "dismissed")
			manual := add("manual", "manual:P0Y8LQ", "#P0Y8LQ", "active")
			reviewed.Revision = 2
			reviewed.AutoRevision = 0
			reviewed.Notes = "Decisão da liderança"
			payload, _ := json.Marshal(reviewed)
			if _, err := db.Exec(`UPDATE moderation_records SET revision=2,payload=? WHERE id=?`, string(payload), reviewed.ID); err != nil {
				t.Fatal(err)
			}
			counts := map[string]finalAttackCount{raid.EventKey: {3, 5}, war.EventKey: {1, 2}, cwl.EventKey: {1, 1}, reviewed.EventKey: {5, 5}, removed.EventKey: {5, 5}, manual.EventKey: {2, 2}, editable.EventKey: {1, 5}}
			candidate := raid
			candidate.Reason = "Contagem final: 3/5 ataques usados."
			editableCandidate := editable
			editableCandidate.Reason = "Contagem final: 1/5 ataques usados."
			conflicts, err := reconcileAutomaticWarnings(ctx, db, "P0Y8LQ", []moderationRecord{candidate, editableCandidate}, counts)
			if err != nil || conflicts != 1 {
				t.Fatal(conflicts, err)
			}
			read := func() map[string]moderationRecord {
				items, err := readModerationRecords(ctx, db, "P0Y8LQ")
				if err != nil {
					t.Fatal(err)
				}
				byKey := map[string]moderationRecord{}
				for _, r := range items {
					byKey[r.EventKey] = r
				}
				return byKey
			}
			items := read()
			if r := items[raid.EventKey]; r.Reason != candidate.Reason || r.Status != "pending" || r.Revision != 2 {
				t.Fatal("raid facts not refreshed", r)
			}
			for _, key := range []string{war.EventKey, cwl.EventKey} {
				if r := items[key]; r.Status != "resolved" || !strings.Contains(r.Notes, "ataques confirmados") {
					t.Fatal("nonzero war still counted", r)
				}
			}
			if r := items[reviewed.EventKey]; r.Status != "active" || r.Notes != "Decisão da liderança" || r.Revision != 2 {
				t.Fatal("leadership decision overwritten", r)
			}
			if items[removed.EventKey].Status != "dismissed" || items[manual.EventKey].Status != "active" {
				t.Fatal("removed or manual record changed")
			}
			// A second automatic correction is still allowed until a human edits it.
			candidate.Reason = "Contagem final: 4/5 ataques usados."
			counts[raid.EventKey] = finalAttackCount{4, 5}
			if _, err := reconcileAutomaticWarnings(ctx, db, "P0Y8LQ", []moderationRecord{candidate, editableCandidate}, counts); err != nil {
				t.Fatal(err)
			}
			if r := read()[raid.EventKey]; r.Reason != candidate.Reason || r.Revision != 3 {
				t.Fatal("automatic revision mistaken for human review", r)
			}
			// A form opened before a machine correction must request a reload,
			// rather than reject the user's unchanged old evidence as an edit.
			staleApp := &app{db: db, clanTags: map[string]string{"1": "P0Y8LQ"}}
			stale := map[string]any{"revision": editable.Revision, "playerTag": editable.PlayerTag, "playerName": editable.PlayerName, "kind": editable.Kind, "sector": editable.Sector, "status": "pending", "reason": editable.Reason, "notes": "Revisão desatualizada"}
			staleResponse := adminTestRequest(adminTestRouter(staleApp), "PUT", "https://dashboard.imperiobarbaro.com.br/api/moderation/"+editable.ID+"?clan=1", stale, true)
			if staleResponse.Code != 409 {
				t.Fatal("stale evidence was not identified as a conflict", staleResponse.Code, staleResponse.Body.String())
			}
			// The real edit endpoint clears automatic ownership, including after
			// prior machine revisions, so subsequent syncs preserve human input.
			current := read()[editable.EventKey]
			a := &app{db: db, clanTags: map[string]string{"1": "P0Y8LQ"}}
			input := map[string]any{"revision": current.Revision, "playerTag": current.PlayerTag, "playerName": current.PlayerName, "kind": current.Kind, "sector": current.Sector, "status": "pending", "reason": current.Reason, "notes": "Conferido pela liderança"}
			response := adminTestRequest(adminTestRouter(a), "PUT", "https://dashboard.imperiobarbaro.com.br/api/moderation/"+current.ID+"?clan=1", input, true)
			if response.Code != 200 {
				t.Fatal(response.Code, response.Body.String())
			}
			counts[editable.EventKey] = finalAttackCount{5, 5}
			counts[raid.EventKey] = finalAttackCount{5, 5}
			if _, err := reconcileAutomaticWarnings(ctx, db, "P0Y8LQ", nil, counts); err != nil {
				t.Fatal(err)
			}
			if r := read()[editable.EventKey]; r.Status != "pending" || r.Notes != "Conferido pela liderança" || r.AutoRevision != 0 {
				t.Fatal("human edit lost after automatic revisions", r)
			}
			if r := read()[raid.EventKey]; r.Status != "resolved" || !strings.Contains(r.Notes, "5/5") {
				t.Fatal("completed raid retained a warning", r)
			}
		})
	}
}
