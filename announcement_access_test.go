package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func scopedRequest(h http.Handler, path, header, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "https://dashboard.imperiobarbaro.com.br"+path, strings.NewReader(body))
	r.SetBasicAuth("leader", "private-admin-test-password")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Clanboard-Request", "1")
	r.Header.Set(header, token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAnnouncementCapabilitiesAreIsolatedEvenWithIdenticalPasswords(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprint("postgres=", postgres), func(t *testing.T) {
			a := &app{db: integrationDatabase(t, postgres)}
			if err := a.initAnnouncementAccess(vaultTestPassword, vaultTestPassword, testClanAnnouncementPasswords()); err != nil {
				t.Fatal(err)
			}
			contacts := newContactVault(vaultTestPassword)
			if postgres {
				if err := contacts.useSharedStore(a.db, vaultTestPassword); err != nil {
					t.Fatal(err)
				}
			}
			vaults := []*contactVault{contacts, a.customVault, a.timerVault}
			for i, source := range vaults {
				w, token := unlockVault(t, source, vaultTestPassword, "192.0.2.1")
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
				for j, target := range vaults {
					r := httptest.NewRequest("POST", "/protected", nil)
					r.Header.Set(target.header, token)
					w := httptest.NewRecorder()
					target.protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
					want := 423
					if i == j {
						want = 204
					}
					if w.Code != want {
						t.Fatalf("scope %s to %s: %d", source.scope, target.scope, w.Code)
					}
				}
			}
		})
	}
}

func TestTimerReleaseRequiresOwnPasswordAndResetsOnlySelectedButton(t *testing.T) {
	a, h := notificationTestApp(t)
	clanToken := testClanAnnouncementToken(t, a, "1")
	enrollPush(t, h, "1")
	for _, kind := range []string{"raids", "games"} {
		if w := announcementTestRequest(h, "/api/announcements", clanToken, map[string]string{"clanId": "1", "kind": kind}); w.Code != 202 {
			t.Fatal(w.Code)
		}
	}
	_, contactToken := unlockVault(t, a.contactsVault, "contacts-test-private-password", "192.0.2.1")
	_, customToken := unlockVault(t, a.customVault, "custom-test-private-password", "192.0.2.1")
	_, timerToken := unlockVault(t, a.timerVault, "timer-test-private-password", "192.0.2.1")
	path, body := "/api/announcements/timer/reset", map[string]string{"clanId": "1", "kind": "raids"}
	for _, token := range []string{"", contactToken, customToken} {
		if w := announcementTestRequest(h, path, clanToken, body, "X-Super-Notifications-Session", token); w.Code != 423 {
			t.Fatalf("unprotected reset: %d", w.Code)
		}
	}
	if w := scopedRequest(h, "/api/announcements/custom", "X-Notifications-Session", timerToken, `{"clanId":"1","kind":"custom","title":"T","body":"B"}`); w.Code != 423 {
		t.Fatal("timer session authorized custom send")
	}
	if w := announcementTestRequest(h, path, clanToken, body, "X-Super-Notifications-Session", timerToken); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if pushCount(t, a, `SELECT COUNT(*) FROM announcements`) != 2 {
		t.Fatal("reset broadcast a message")
	}
	if w := announcementTestRequest(h, "/api/announcements", clanToken, map[string]string{"clanId": "1", "kind": "raids"}); w.Code != 202 {
		t.Fatal(w.Code)
	}
	for _, kind := range []string{"raids", "games"} {
		if w := announcementTestRequest(h, "/api/announcements", clanToken, map[string]string{"clanId": "1", "kind": kind}); w.Code != 429 {
			t.Fatal("cooldown lost", kind, w.Code)
		}
	}
	for _, bad := range []string{`{"clanId":"4","kind":"raids"}`, `{"clanId":"1","kind":"unknown"}`} {
		if w := scopedRequest(h, path, "X-Super-Notifications-Session", timerToken, bad); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	// Timer credentials never waive server-side war eligibility.
	if w := announcementTestRequest(h, path, clanToken, map[string]string{"clanId": "1", "kind": "war-second"}, "X-Super-Notifications-Session", timerToken); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := announcementTestRequest(h, "/api/announcements", clanToken, map[string]string{"clanId": "1", "kind": "war-second"}); w.Code != 409 {
		t.Fatal("war guard bypassed")
	}
	a.timerVault.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	if w := announcementTestRequest(h, path, clanToken, body, "X-Super-Notifications-Session", timerToken); w.Code != 423 {
		t.Fatal("expired timer session accepted")
	}
}

func TestNewPasswordsFailClosedAndDoNotAcceptContactPassword(t *testing.T) {
	a, h := notificationTestApp(t)
	for _, path := range []string{"custom", "timer"} {
		w := scopedRequest(h, "/api/announcements/"+path+"/unlock", "X-Clanboard-Request", "1", `{"password":"contacts-test-private-password"}`)
		if w.Code != 403 {
			t.Fatal(path, w.Code)
		}
		var value map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &value)
		if value["token"] != nil {
			t.Fatal("wrong password created session")
		}
	}
	if err := a.initAnnouncementAccess("", "", nil); err != nil {
		t.Fatal(err)
	}
	for _, v := range []*contactVault{a.customVault, a.timerVault} {
		w, _ := unlockVault(t, v, "custom-test-private-password", "192.0.2.2")
		if w.Code != 503 {
			t.Fatal("unconfigured password accepted")
		}
	}
}

func TestAnnouncementPasswordIsBoundToTargetClan(t *testing.T) {
	a, h := notificationTestApp(t)
	enrollPush(t, h, "all")
	passwords := testClanAnnouncementPasswords()
	path := "/api/announcements/clans/3"
	const base = "https://dashboard.imperiobarbaro.com.br"
	wrong := adminTestRequest(h, "POST", base+path+"/unlock", map[string]string{"password": passwords["1"]}, true)
	if wrong.Code != 403 {
		t.Fatalf("another clan's password unlocked clan 3: %d", wrong.Code)
	}
	correct := adminTestRequest(h, "POST", base+path+"/unlock", map[string]string{"password": passwords["3"]}, true)
	var session struct{ Token string }
	if err := json.Unmarshal(correct.Body.Bytes(), &session); err != nil || correct.Code != 200 || session.Token == "" {
		t.Fatalf("clan unlock: %d %s", correct.Code, correct.Body)
	}
	_, custom := unlockVault(t, a.customVault, "custom-test-private-password", "192.0.2.1")
	_, timer := unlockVault(t, a.timerVault, "timer-test-private-password", "192.0.2.1")
	for _, c := range []struct {
		path, scope, token, kind string
	}{
		{"/api/announcements", clanAnnouncementHeader, session.Token, "raids"},
		{"/api/announcements/custom", "X-Notifications-Session", custom, "custom"},
		{"/api/announcements/timer/reset", "X-Super-Notifications-Session", timer, "raids"},
	} {
		body := map[string]string{"clanId": "1", "kind": c.kind}
		if c.kind == "custom" {
			body["title"], body["body"] = "Aviso", "Mensagem"
		}
		if res := announcementTestRequest(h, c.path, session.Token, body, c.scope, c.token); res.Code != 423 {
			t.Fatalf("clan 3 session accepted for clan 1 at %s: %d", c.path, res.Code)
		}
	}
	body := map[string]string{"clanId": "3", "kind": "raids"}
	if res := announcementTestRequest(h, "/api/announcements", "", body); res.Code != 423 {
		t.Fatalf("publishing without clan session: %d", res.Code)
	}
	if res := announcementTestRequest(h, "/api/announcements", session.Token, body); res.Code != 202 {
		t.Fatalf("publishing with clan session: %d %s", res.Code, res.Body)
	}
	for _, c := range []struct {
		clan, token string
		code        int
		history     bool
	}{
		{"3", "", 200, false}, {"3", session.Token, 200, true}, {"1", session.Token, 423, false},
	} {
		req := httptest.NewRequest("GET", base+"/api/announcements?clan="+c.clan, nil)
		req.SetBasicAuth("leader", "private-admin-test-password")
		req.Header.Set(clanAnnouncementHeader, c.token)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != c.code || strings.Contains(res.Body.String(), `"items"`) != c.history {
			t.Fatalf("unexpected history response: %d %s", res.Code, res.Body)
		}
	}
	req := httptest.NewRequest("DELETE", base+path+"/session", nil)
	req.SetBasicAuth("leader", "private-admin-test-password")
	req.Header.Set("X-Clanboard-Request", "1")
	req.Header.Set(clanAnnouncementHeader, session.Token)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != 204 || announcementTestRequest(h, "/api/announcements", session.Token, body).Code != 423 {
		t.Fatal("clan session was not revoked")
	}
}

func TestClanAnnouncementPasswordsMustBeDistinct(t *testing.T) {
	passwords := testClanAnnouncementPasswords()
	if err := validateDedicatedPasswords(passwords, "private-dashboard-password"); err != nil {
		t.Fatal(err)
	}
	if err := validateDedicatedPasswords(passwords, passwords["3"]); err == nil {
		t.Fatal("a clan reused a password from another part of the dashboard")
	}
	passwords["3"] = passwords["2"]
	if err := validateDedicatedPasswords(passwords); err == nil {
		t.Fatal("two clans used the same password")
	}
}
