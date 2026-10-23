package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *app) initAnnouncementAccess(customPassword, timerPassword string) {
	a.customVault = newScopedVault(customPassword, "notifications", "X-Notifications-Session", "NOTIFICATIONS_PASSWORD")
	a.timerVault = newScopedVault(timerPassword, "super-notifications", "X-Super-Notifications-Session", "SUPER_NOTIFICATIONS_PASSWORD")
	if a.db.postgres {
		a.customVault.store = a.db
		a.timerVault.store = a.db
	}
}

func (a *app) announcementAccessRoutes(r chi.Router) {
	r.Post("/api/announcements/custom/unlock", a.customVault.unlock)
	r.Delete("/api/announcements/custom/session", a.customVault.lock)
	r.With(a.customVault.protect).Post("/api/announcements/custom", a.sendAnnouncement)
	r.Post("/api/announcements/timer/unlock", a.timerVault.unlock)
	r.Delete("/api/announcements/timer/session", a.timerVault.lock)
	r.With(a.timerVault.protect).Post("/api/announcements/timer/reset", a.resetAnnouncementTimer)
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
	if (input.ClanID != "1" && input.ClanID != "2") || !validKind {
		securityError(w, 400, "Escolha o clã e um dos avisos disponíveis.")
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE announcement_cooldowns SET next_at=0 WHERE clan_id=? AND kind=?`, input.ClanID, input.Kind); err != nil {
		securityError(w, 500, "Não foi possível liberar o intervalo.")
		return
	}
	jsonResponse(w, 200, map[string]any{"nextAt": 0})
}
