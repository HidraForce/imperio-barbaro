package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type finalAttackCount struct{ used, limit int }

func finalWarCounts(body []byte, tag, source, event string, since, now time.Time, counts map[string]finalAttackCount) {
	var war moderationWar
	if json.Unmarshal(body, &war) != nil || war.State != "warEnded" || !finishedPeriod(war.StartTime, war.EndTime, since, now) {
		return
	}
	side, limit, ok := finalWarSide(war, tag, source)
	if !ok {
		return
	}
	for _, member := range side.Members {
		counts[source+":"+event+":"+normalizeTag(member.Tag)] = finalAttackCount{len(member.Attacks), limit}
	}
}

func finalRaidCounts(body []byte, since, now time.Time, counts map[string]finalAttackCount) {
	var response struct {
		Items []moderationRaidSeason `json:"items"`
	}
	if json.Unmarshal(body, &response) != nil {
		return
	}
	for _, season := range response.Items {
		if season.State != "ended" || !finishedPeriod(season.StartTime, season.EndTime, since, now) || !finalRaidMembers(season) {
			continue
		}
		for _, member := range season.Members {
			if member.AttackLimit == nil || *member.AttackLimit < 1 || member.Bonus < 0 || member.Bonus > 1 {
				continue
			}
			limit := *member.AttackLimit + member.Bonus
			if limit > 6 || *member.Attacks > limit {
				continue
			}
			counts["raids:"+season.StartTime+":"+normalizeTag(member.Tag)] = finalAttackCount{*member.Attacks, limit}
		}
	}
}

// Only untouched automatic occurrences may be corrected. The revision predicate
// also protects a leader's decision made concurrently with the final check.
func reconcileAutomaticWarnings(ctx context.Context, db *database, tag string, candidates []moderationRecord, counts map[string]finalAttackCount) (int, error) {
	if len(candidates) == 0 && len(counts) == 0 {
		return 0, nil
	}
	stored, err := readModerationRecords(ctx, db, tag)
	if err != nil {
		return 0, err
	}
	expected := map[string]moderationRecord{}
	for _, candidate := range candidates {
		expected[candidate.EventKey] = candidate
	}
	conflicts := 0
	for _, record := range stored {
		if record.Kind != "warning" || record.Source == "manual" || (record.Status != "pending" && record.Status != "active") {
			continue
		}
		candidate, wanted := expected[record.EventKey]
		count, known := counts[record.EventKey]
		resolved := known && !wanted && ((record.Source == "raids" && count.used >= count.limit) || ((record.Source == "war" || record.Source == "cwl") && count.used > 0))
		refresh := wanted && ((record.Status == "pending" && record.Reason != candidate.Reason) || (record.Status == "active" && candidate.Status == "pending"))
		if !resolved && !refresh {
			continue
		}
		if record.Revision != 1 && record.AutoRevision != record.Revision {
			if resolved {
				conflicts++
			}
			continue
		}
		if resolved {
			record.Status = "resolved"
			if record.Notes != "" {
				record.Notes += "\n"
			}
			record.Notes += fmt.Sprintf("Revisão automática após o encerramento: %d/%d ataques confirmados. Esta ocorrência não conta como aviso.", count.used, count.limit)
		} else {
			record.Reason = candidate.Reason
			record.EventStart, record.EventEnd = candidate.EventStart, candidate.EventEnd
			if candidate.Status == "pending" {
				record.Status = "pending"
			}
		}
		previous := record.Revision
		record.Revision++
		record.AutoRevision = record.Revision
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		payload, _ := json.Marshal(record)
		if _, err = db.ExecContext(ctx, `UPDATE moderation_records SET revision=revision+1,payload=? WHERE id=? AND clan_tag=? AND revision=?`, string(payload), record.ID, tag, previous); err != nil {
			return conflicts, err
		}
	}
	return conflicts, nil
}
