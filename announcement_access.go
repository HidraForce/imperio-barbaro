package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

const clanAnnouncementHeader = "X-Clan-Announcements-Session"

func clanAnnouncementPasswordName(id string) string { return "COMS_C" + id }

func newClanAnnouncementVault(id, password string) *contactVault {
	return newScopedVault(password, "clan-announcements-"+id, clanAnnouncementHeader, clanAnnouncementPasswordName(id))
}

func validateClanAnnouncementPasswords(passwords map[string]string, otherPasswords ...string) error {
	used := make(map[string]bool)
	for _, password := range otherPasswords {
		used[password] = true
	}
	for _, clan := range communityClans {
		password := passwords[clan.ID]
		if password == "" {
			continue
		}
		if !validCredentials("avisos", password) || used[password] {
			return fmt.Errorf("%s deve ter pelo menos 16 caracteres e ser diferente das outras senhas do aplicativo", clanAnnouncementPasswordName(clan.ID))
		}
		used[password] = true
	}
	return nil
}

func (a *app) initAnnouncementAccess(customPassword, timerPassword string, clanPasswords map[string]string) error {
	a.customVault = newScopedVault(customPassword, "notifications", "X-Notifications-Session", "NOTIFICATIONS_PASSWORD")
	a.timerVault = newScopedVault(timerPassword, "super-notifications", "X-Super-Notifications-Session", "SUPER_NOTIFICATIONS_PASSWORD")
	a.clanAnnouncementVaults = make(map[string]*contactVault, len(communityClans))
	for _, clan := range communityClans {
		password := clanPasswords[clan.ID]
		vault := newClanAnnouncementVault(clan.ID, password)
		if a.db.postgres && vault.configured {
			if err := vault.useSharedStore(a.db, password); err != nil {
				return err
			}
		}
		a.clanAnnouncementVaults[clan.ID] = vault
	}
	if a.db.postgres {
		if err := a.customVault.useSharedStore(a.db, customPassword); err != nil {
			return err
		}
		if err := a.timerVault.useSharedStore(a.db, timerPassword); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) announcementAccessRoutes(r chi.Router) {
	r.Get("/api/announcements", a.announcementStatus)
	r.Get("/api/announcements/audience", a.announcementAudience)
	r.Post("/api/announcements", a.sendAnnouncement)
	for _, clan := range communityClans {
		vault := a.clanAnnouncementVaults[clan.ID]
		path := "/api/announcements/clans/" + clan.ID
		r.Post(path+"/unlock", vault.unlock)
		r.Delete(path+"/session", vault.lock)
	}
	r.Post("/api/announcements/custom/unlock", a.customVault.unlock)
	r.Delete("/api/announcements/custom/session", a.customVault.lock)
	r.With(a.customVault.protect).Post("/api/announcements/custom", a.sendAnnouncement)
	r.Post("/api/announcements/timer/unlock", a.timerVault.unlock)
	r.Delete("/api/announcements/timer/session", a.timerVault.lock)
	r.With(a.timerVault.protect).Post("/api/announcements/timer/reset", a.resetAnnouncementTimer)
}

func (a *app) authorizeClanAnnouncement(w http.ResponseWriter, r *http.Request, id string) bool {
	vault := a.clanAnnouncementVaults[id]
	if vault == nil {
		securityError(w, 503, "A senha de comunicados deste clã ainda não foi configurada.")
		return false
	}
	return vault.authorize(w, r)
}

// Releasing a single button never sends a message or bypasses the war rules.
// The next successful send starts its usual one-hour cooldown again.
func (a *app) resetAnnouncementTimer(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ClanID string `json:"clanId"`
		Kind   string `json:"kind"`
	}
	if !decodeAdminJSON(w, r, &input) {
		return
	}
	validKind := input.Kind == "custom"
	for _, item := range announcementTemplates {
		validKind = validKind || item.Kind == input.Kind
	}
	if _, valid := findCommunityClan(input.ClanID); !valid || !validKind {
		securityError(w, 400, "Escolha o clã e um dos avisos disponíveis.")
		return
	}
	if !a.authorizeClanAnnouncement(w, r, input.ClanID) {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE announcement_cooldowns SET next_at=0 WHERE clan_id=? AND kind=?`, input.ClanID, input.Kind); err != nil {
		securityError(w, 500, "Não foi possível liberar o intervalo.")
		return
	}
	jsonResponse(w, 200, map[string]any{"nextAt": 0})
}
