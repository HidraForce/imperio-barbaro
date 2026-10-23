package main

import "time"

func minSnapshotTime(now, fetchedAt time.Time) time.Time {
	if fetchedAt.Before(now) {
		return fetchedAt
	}
	return now
}

func finishedPeriod(start, end string, since, now time.Time) bool {
	startAt, startErr := time.Parse("20060102T150405.000Z", start)
	endAt, endErr := time.Parse("20060102T150405.000Z", end)
	return startErr == nil && endErr == nil && startAt.Before(endAt) && closedSince(end, since, now)
}

// Missing attack arrays are normal for zero-attack war members. Confirm the
// complete lineup and its aggregate before interpreting an absent array as zero.
func finalWarSide(war moderationWar, tag, source string) (moderationSide, int, bool) {
	side := war.Clan
	if normalizeTag(side.Tag) != tag {
		side = war.Opponent
	}
	limit := war.AttacksPerMember
	if source == "cwl" && limit == 0 {
		limit = 1 // The CWL response does not include attacksPerMember.
	}
	if normalizeTag(side.Tag) != tag || war.TeamSize < 1 || len(side.Members) != war.TeamSize || side.Attacks == nil || (source != "war" && source != "cwl") || (source == "war" && limit != 2) || (source == "cwl" && limit != 1) {
		return side, limit, false
	}
	seen, total := map[string]bool{}, 0
	for _, member := range side.Members {
		key := "#" + normalizeTag(member.Tag)
		if !playerTagPattern.MatchString(key) || seen[key] || len(member.Attacks) > limit {
			return side, limit, false
		}
		seen[key] = true
		total += len(member.Attacks)
	}
	return side, limit, total == *side.Attacks
}

type moderationRaidMember struct {
	Tag         string `json:"tag"`
	Name        string `json:"name"`
	Attacks     *int   `json:"attacks"`
	AttackLimit *int   `json:"attackLimit"`
	Bonus       int    `json:"bonusAttackLimit"`
}

type moderationRaidSeason struct {
	State        string                 `json:"state"`
	StartTime    string                 `json:"startTime"`
	EndTime      string                 `json:"endTime"`
	TotalAttacks *int                   `json:"totalAttacks"`
	Members      []moderationRaidMember `json:"members"`
}

func finalRaidMembers(season moderationRaidSeason) bool {
	if season.TotalAttacks == nil || *season.TotalAttacks < 0 {
		return false
	}
	seen, total := map[string]bool{}, 0
	for _, member := range season.Members {
		key := "#" + normalizeTag(member.Tag)
		if !playerTagPattern.MatchString(key) || seen[key] || member.Attacks == nil || *member.Attacks < 0 || *member.Attacks > 6 {
			return false
		}
		seen[key] = true
		total += *member.Attacks
	}
	// members may be omitted when totalAttacks explicitly confirms zero.
	return total == *season.TotalAttacks
}
