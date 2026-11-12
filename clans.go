package main

import "os"

type communityClan struct {
	ID                string
	Name              string
	Label             string
	ProfileTag        string
	RulesAnchor       string
	Description       string
	AnnouncementLabel string
}

var communityClans = [...]communityClan{
	{
		ID: "1", Name: "Império Bárbaro", Label: "Clã principal", ProfileTag: "2J88098Q0", RulesAnchor: "regras-principal",
		Description:       "Compromisso com as guerras, organização na liga e participação nas metas do clã.",
		AnnouncementLabel: "Principal",
	},
	{
		ID: "2", Name: "Império Bárbar2", Label: "Clã secundário", ProfileTag: "2RJY0VC8C", RulesAnchor: "regras-secundario",
		Description: "Uma academia para aprender ataques, recuperar a vila e crescer junto com a equipe.",
	},
	{
		ID: "3", Name: "Clã 3", Label: "Clã terciário", ProfileTag: "2CJR2VL8R", RulesAnchor: "regras-cla-3",
	},
}

func findCommunityClan(id string) (communityClan, bool) {
	for _, clan := range communityClans {
		if clan.ID == id {
			return clan, true
		}
	}
	return communityClan{}, false
}

func configuredClanTags() map[string]string {
	tags := make(map[string]string, len(communityClans))
	for _, clan := range communityClans {
		tag, configured := os.LookupEnv("CLAN_TAG" + clan.ID)
		if !configured && clan.ID == "3" {
			tag = clan.ProfileTag
		}
		tags[clan.ID] = normalizeTag(tag)
	}
	return tags
}
