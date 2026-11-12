package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type communityPushClient struct {
	mu    sync.Mutex
	calls map[string]int
}

func (c *communityPushClient) Do(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.calls[r.URL.String()]++
	c.mu.Unlock()
	return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func TestAllClansReachEveryDeviceIncludingLegacyEnrollments(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprint("postgres=", postgres), func(t *testing.T) {
			a := &app{db: integrationDatabase(t, postgres)}
			var err error
			a.notifications, err = initNotifications(a.db)
			if err != nil {
				t.Fatal(err)
			}
			h := a.publicPush(http.NotFoundHandler())
			mainDevice := enrollPush(t, h, "1")
			academyDevice := enrollPush(t, h, "2")
			communityDevice := testPushSubscription(t)
			input := map[string]any{"subscription": communityDevice}
			for range 2 {
				res := adminTestRequest(h, "POST", "https://imperiobarbaro.com.br/api/push/subscription", input, false)
				if res.Code != 200 {
					t.Fatalf("enrollment without clan failed: %d %s", res.Code, res.Body.String())
				}
			}
			if pushCount(t, a, `SELECT COUNT(*) FROM push_subscriptions`) != 3 || pushCount(t, a, `SELECT COUNT(*) FROM push_subscriptions WHERE clan_id='all'`) != 1 {
				t.Fatal("enrollment required a clan or duplicated a device")
			}
			for _, clan := range []string{"1", "2", "3"} {
				res := adminTestRequest(http.HandlerFunc(a.sendAnnouncement), "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": clan, "kind": "raids"}, true)
				if res.Code != 202 {
					t.Fatalf("publish %s: %d %s", clan, res.Code, res.Body.String())
				}
				var queued struct {
					Queued int `json:"queued"`
				}
				if err = json.Unmarshal(res.Body.Bytes(), &queued); err != nil || queued.Queued != 3 {
					t.Fatalf("clan %s did not include all devices: %s", clan, res.Body.String())
				}
				status := adminTestRequest(http.HandlerFunc(a.announcementStatus), "GET", "https://dashboard.imperiobarbaro.com.br/api/announcements?clan="+clan, nil, true)
				var reach struct {
					Subscribers int    `json:"subscribers"`
					ClanName    string `json:"clanName"`
				}
				if json.Unmarshal(status.Body.Bytes(), &reach) != nil || reach.Subscribers != 3 || reach.ClanName != announcementClanName(clan) {
					t.Fatalf("incorrect audience: %s", status.Body.String())
				}
			}
			client := &communityPushClient{calls: make(map[string]int)}
			a.notifications.client = client
			a.dispatchPush(context.Background())
			for _, device := range []string{mainDevice.Endpoint, academyDevice.Endpoint, communityDevice.Endpoint} {
				if client.calls[device] != 3 {
					t.Fatalf("device received %d of three clan notices", client.calls[device])
				}
			}
			if pushCount(t, a, `SELECT COUNT(*) FROM push_jobs WHERE state='accepted'`) != 9 {
				t.Fatal("cross-clan jobs were dropped")
			}
			if _, err = a.db.Exec(`UPDATE announcements SET created_at=? WHERE clan_id='1'`, time.Now().Unix()-60); err != nil {
				t.Fatal(err)
			}
			if _, err = a.db.Exec(`UPDATE announcements SET created_at=? WHERE clan_id='2'`, time.Now().Unix()-30); err != nil {
				t.Fatal(err)
			}
			feed := adminTestRequest(h, "GET", "https://imperiobarbaro.com.br/api/push/announcements", nil, false)
			var result struct {
				Items []struct {
					ClanID   string `json:"clanId"`
					ClanName string `json:"clanName"`
				} `json:"items"`
			}
			if feed.Code != 200 || json.Unmarshal(feed.Body.Bytes(), &result) != nil || len(result.Items) != 3 {
				t.Fatalf("combined feed missing notices: %d %s", feed.Code, feed.Body.String())
			}
			if result.Items[0].ClanID != "3" || result.Items[1].ClanID != "2" || result.Items[2].ClanID != "1" {
				t.Fatal("combined feed is not chronological")
			}
			for _, item := range result.Items {
				if item.ClanName != announcementClanName(item.ClanID) {
					t.Fatal("feed did not identify the clan")
				}
			}
			for _, private := range []string{"endpoint", "recipients", "p256dh", "auth"} {
				if strings.Contains(feed.Body.String(), `"`+private+`"`) {
					t.Fatal("combined feed exposed private data")
				}
			}
		})
	}
}

func TestAnnouncementPayloadAlwaysNamesItsClan(t *testing.T) {
	for _, clan := range []struct{ id, label string }{
		{"1", "Império Bárbaro · Principal"},
		{"2", "Império Bárbar2"},
		{"3", "Império Bárbaro 3"},
	} {
		for _, kind := range []string{"raids", "war-zero", "war-second", "games", "cwl", "custom"} {
			item := announcement{ID: "notice", ClanID: clan.id, Kind: kind, Title: "Aviso", Body: "Use seus ataques."}
			payload := announcementPushPayload(item)
			if payload["title"] != clan.label+" · Aviso" || payload["body"] != "Clã: "+clan.label+"\nUse seus ataques." || payload["clanId"] != clan.id || payload["clanName"] != clan.label || payload["url"] != "/app#notice" {
				t.Fatalf("clan missing in %s payload: %+v", kind, payload)
			}
		}
	}
}

func TestSelectedClansAndAuthenticatedSenderReceiveOnePushPerDevice(t *testing.T) {
	a, handler := notificationTestApp(t)
	const dashboard = "https://dashboard.imperiobarbaro.com.br"
	const community = "https://imperiobarbaro.com.br"
	enroll := func(origin string, subscription webpush.Subscription, clans []string, status int) {
		t.Helper()
		res := adminTestRequest(handler, "POST", origin+"/api/push/subscription", map[string]any{"subscription": subscription, "clanIds": clans}, origin == dashboard)
		if res.Code != status {
			t.Fatalf("enrollment: %d %s", res.Code, res.Body)
		}
	}
	publish := func(clan, kind string, sender webpush.Subscription, status, queued int) string {
		t.Helper()
		res := adminTestRequest(handler, "POST", dashboard+"/api/announcements", map[string]any{"clanId": clan, "kind": kind, "senderSubscription": sender}, true)
		if res.Code != status {
			t.Fatalf("publish: %d %s", res.Code, res.Body)
		}
		if status != 202 {
			return ""
		}
		var result struct {
			ID     string
			Queued int
		}
		if json.Unmarshal(res.Body.Bytes(), &result) != nil || result.ID == "" || result.Queued != queued {
			t.Fatalf("unexpected receipt: %s", res.Body)
		}
		return result.ID
	}
	sender, multiClanMember, secondClanMember := testPushSubscription(t), testPushSubscription(t), testPushSubscription(t)
	enroll(dashboard, sender, []string{"2"}, 200)
	forged := sender
	forged.Keys = testPushSubscription(t).Keys
	publish("3", "games", forged, 409, 0)
	publish("3", "games", sender, 202, 1)
	enroll(community, multiClanMember, []string{"1", "2", "3"}, 200)
	enroll(community, multiClanMember, []string{"3", "1"}, 200)
	enroll(community, multiClanMember, []string{"3", "3"}, 400)
	enroll(community, multiClanMember, []string{"4"}, 400)
	enroll(community, multiClanMember, []string{}, 400)
	enroll(community, multiClanMember, nil, 200)
	restore := adminTestRequest(handler, "POST", community+"/api/push/subscription/restore", map[string]any{"subscription": multiClanMember}, false)
	if restore.Code != 200 || strings.TrimSpace(restore.Body.String()) != `{"clanId":"all","clanIds":["1","3"]}` {
		t.Fatalf("unexpected restored preferences: %s", restore.Body)
	}
	enroll(community, secondClanMember, []string{"2"}, 200)
	id := publish("3", "raids", sender, 202, 2)
	publish("2", "raids", sender, 202, 2)
	feed := adminTestRequest(handler, "GET", dashboard+"/api/push/announcements?clan=3", nil, true)
	if feed.Code != 200 || !strings.Contains(feed.Body.String(), id) {
		t.Fatalf("sent notice missing on the dashboard origin: %s", feed.Body)
	}
	client := &communityPushClient{calls: make(map[string]int)}
	a.notifications.client = client
	a.dispatchPush(context.Background())
	if client.calls[sender.Endpoint] != 3 || client.calls[multiClanMember.Endpoint] != 1 || client.calls[secondClanMember.Endpoint] != 1 {
		t.Fatalf("unexpected delivery counts: sender %d, multi-clan member %d, second-clan member %d",
			client.calls[sender.Endpoint], client.calls[multiClanMember.Endpoint], client.calls[secondClanMember.Endpoint])
	}
	if announcementClanName("2") != "Império Bárbar2" {
		t.Fatal("unexpected second-clan label")
	}
}
