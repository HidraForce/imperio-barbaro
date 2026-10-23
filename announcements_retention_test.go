package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func seedAnnouncementHistory(t *testing.T, db *database, id, clan string, created int64, state string, lease int64) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO announcements(id,clan_id,kind,title,body,created_at,recipients) VALUES(?,?,'raids',?,'Mensagem de teste',?,1)`, id, clan, id, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO push_jobs(id,announcement_id,subscription_id,state,next_at,lease_until) VALUES(?,?,'retention-device',?,0,?)`, "job-"+id, id, state, lease); err != nil {
		t.Fatal(err)
	}
}

func TestAnnouncementRetentionKeepsTenPerClanAndProtectsDeliveries(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%v", postgres), func(t *testing.T) {
			db := integrationDatabase(t, postgres)
			service, err := initNotifications(db)
			if err != nil {
				t.Fatal(err)
			}
			a := &app{db: db}
			now := time.Now().Unix()
			for _, clan := range []string{"1", "2"} {
				for i := 0; i < 14; i++ {
					state, lease := "accepted", int64(0)
					if clan == "1" && i == 1 {
						state = "pending"
					}
					if clan == "1" && i == 2 {
						lease = now + 60
					}
					seedAnnouncementHistory(t, db, fmt.Sprintf("%s-%02d", clan, i), clan, now-300+int64(i), state, lease)
				}
			}
			if _, err = db.Exec(`INSERT INTO announcement_cooldowns(clan_id,kind,next_at) VALUES('1','raids',?)`, now+3600); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO push_subscriptions(id,endpoint,p256dh,auth,clan_id,updated_at) VALUES('retention-device','https://fcm.googleapis.com/fcm/send/retention-test','key','auth','1',?)`, now); err != nil {
				t.Fatal(err)
			}
			if err = maintainAnnouncements(context.Background(), db, now); err != nil {
				t.Fatal(err)
			}
			if pushCount(t, a, `SELECT COUNT(*) FROM announcements WHERE clan_id='1'`) != 12 || pushCount(t, a, `SELECT COUNT(*) FROM announcements WHERE clan_id='2'`) != 10 {
				t.Fatal("retention removed active deliveries or mixed the clans")
			}
			for _, clan := range []string{"1", "2"} {
				items, err := a.announcementList(context.Background(), clan)
				if err != nil || len(items) != 10 || items[0].ID != clan+"-13" || items[9].ID != clan+"-04" {
					t.Fatalf("incorrect visible history for %s: %+v, %v", clan, items, err)
				}
			}
			if pushCount(t, a, `SELECT COUNT(*) FROM push_jobs j LEFT JOIN announcements a ON a.id=j.announcement_id WHERE a.id IS NULL`) != 0 {
				t.Fatal("delivery rows were orphaned")
			}
			if _, err = db.Exec(`UPDATE push_jobs SET state='accepted',lease_until=0 WHERE announcement_id='1-01' OR announcement_id='1-02'`); err != nil {
				t.Fatal(err)
			}
			if err = maintainAnnouncements(context.Background(), db, now); err != nil {
				t.Fatal(err)
			}
			if pushCount(t, a, `SELECT COUNT(*) FROM announcements`) != 20 || pushCount(t, a, `SELECT COUNT(*) FROM push_jobs`) != 20 {
				t.Fatal("old history or its delivery rows were not deleted")
			}
			if pushCount(t, a, `SELECT COUNT(*) FROM push_subscriptions`) != 1 || pushCount(t, a, `SELECT COUNT(*) FROM announcement_cooldowns WHERE next_at>0`) != 1 {
				t.Fatal("retention changed subscriptions or the send cooldown")
			}
			var public, private string
			if err = db.QueryRow(`SELECT public_key,private_key FROM push_keys WHERE id=1`).Scan(&public, &private); err != nil || public != service.publicKey || private != service.privateKey {
				t.Fatal("retention changed the VAPID identity")
			}
		})
	}
}

func TestAnnouncementRetentionResumesOnStartupWithoutInterruptingLeases(t *testing.T) {
	a, _ := notificationTestApp(t)
	now := time.Now().Unix()
	seedAnnouncementHistory(t, a.db, "expired", "1", now-7200, "pending", 0)
	seedAnnouncementHistory(t, a.db, "in-flight", "1", now-7100, "pending", now+60)
	for i := 0; i < 10; i++ {
		seedAnnouncementHistory(t, a.db, fmt.Sprintf("recent-%02d", i), "1", now-100+int64(i), "accepted", 0)
	}
	if _, err := initNotifications(a.db); err != nil {
		t.Fatal(err)
	}
	if pushCount(t, a, `SELECT COUNT(*) FROM announcements WHERE id='expired'`) != 0 || pushCount(t, a, `SELECT COUNT(*) FROM push_jobs WHERE announcement_id='expired'`) != 0 {
		t.Fatal("expired old announcement survived startup cleanup")
	}
	if pushCount(t, a, `SELECT COUNT(*) FROM push_jobs WHERE announcement_id='in-flight' AND state='pending'`) != 1 {
		t.Fatal("startup cleanup interrupted an active lease")
	}
	if err := maintainAnnouncements(context.Background(), a.db, now+61); err != nil {
		t.Fatal(err)
	}
	if pushCount(t, a, `SELECT COUNT(*) FROM announcements`) != 10 || pushCount(t, a, `SELECT COUNT(*) FROM push_jobs`) != 10 {
		t.Fatal("expired lease was not cleaned up")
	}
}

func TestPublishingEleventhAnnouncementDeletesOldestCompletedRecord(t *testing.T) {
	a, handler := notificationTestApp(t)
	enrollPush(t, handler, "1")
	now := time.Now().Unix()
	for i := 0; i < 10; i++ {
		seedAnnouncementHistory(t, a.db, fmt.Sprintf("previous-%02d", i), "1", now-100+int64(i), "accepted", 0)
	}
	res := adminTestRequest(handler, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": "raids"}, true)
	if res.Code != 202 {
		t.Fatalf("publish failed: %d %s", res.Code, res.Body.String())
	}
	if pushCount(t, a, `SELECT COUNT(*) FROM announcements`) != 10 || pushCount(t, a, `SELECT COUNT(*) FROM push_jobs`) != 10 {
		t.Fatal("publishing did not trim the history and delivery rows together")
	}
	if pushCount(t, a, `SELECT COUNT(*) FROM announcements WHERE id='previous-00'`) != 0 || pushCount(t, a, `SELECT COUNT(*) FROM push_jobs WHERE state='pending'`) != 1 {
		t.Fatal("publishing deleted the wrong message or its pending delivery")
	}
	if res = adminTestRequest(handler, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": "raids"}, true); res.Code != 429 {
		t.Fatal("trimming history bypassed the one-hour cooldown")
	}
}
