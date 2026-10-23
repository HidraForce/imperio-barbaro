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
			a.initAnnouncementAccess(vaultTestPassword, vaultTestPassword)
			contacts := newContactVault(vaultTestPassword)
			if postgres {
				contacts.store = a.db
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
	enrollPush(t, h, "1")
	for _, kind := range []string{"raids", "games"} {
		if w := adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": kind}, true); w.Code != 202 {
			t.Fatal(w.Code)
		}
	}
	_, contactToken := unlockVault(t, a.contactsVault, "contacts-test-private-password", "192.0.2.1")
	_, customToken := unlockVault(t, a.customVault, "custom-test-private-password", "192.0.2.1")
	_, timerToken := unlockVault(t, a.timerVault, "timer-test-private-password", "192.0.2.1")
	path, body := "/api/announcements/timer/reset", `{"clanId":"1","kind":"raids"}`
	for _, token := range []string{"", contactToken, customToken} {
		if w := scopedRequest(h, path, "X-Super-Notifications-Session", token, body); w.Code != 423 {
			t.Fatalf("unprotected reset: %d", w.Code)
		}
	}
	if w := scopedRequest(h, "/api/announcements/custom", "X-Notifications-Session", timerToken, `{"clanId":"1","kind":"custom","title":"T","body":"B"}`); w.Code != 423 {
		t.Fatal("timer session authorized custom send")
	}
	if w := scopedRequest(h, path, "X-Super-Notifications-Session", timerToken, body); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if pushCount(t, a, `SELECT COUNT(*) FROM announcements`) != 2 {
		t.Fatal("reset broadcast a message")
	}
	if w := adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": "raids"}, true); w.Code != 202 {
		t.Fatal(w.Code)
	}
	for _, kind := range []string{"raids", "games"} {
		if w := adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": kind}, true); w.Code != 429 {
			t.Fatal("cooldown lost", kind, w.Code)
		}
	}
	for _, bad := range []string{`{"clanId":"3","kind":"raids"}`, `{"clanId":"1","kind":"unknown"}`} {
		if w := scopedRequest(h, path, "X-Super-Notifications-Session", timerToken, bad); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	// Timer credentials never waive server-side war eligibility.
	if w := scopedRequest(h, path, "X-Super-Notifications-Session", timerToken, `{"clanId":"1","kind":"war-second"}`); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": "war-second"}, true); w.Code != 409 {
		t.Fatal("war guard bypassed")
	}
	a.timerVault.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	if w := scopedRequest(h, path, "X-Super-Notifications-Session", timerToken, body); w.Code != 423 {
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
	a.initAnnouncementAccess("", "")
	for _, v := range []*contactVault{a.customVault, a.timerVault} {
		w, _ := unlockVault(t, v, "custom-test-private-password", "192.0.2.2")
		if w.Code != 503 {
			t.Fatal("unconfigured password accepted")
		}
	}
}
