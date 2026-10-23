package main

import "context"

const announcementHistoryLimit = 10

func maintainAnnouncements(ctx context.Context, db *database, now int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = maintainAnnouncementsTx(ctx, tx, now); err != nil {
		return err
	}
	return tx.Commit()
}

func maintainAnnouncementsTx(ctx context.Context, tx *databaseTx, now int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM push_test_limits WHERE next_at<=? OR NOT EXISTS (SELECT 1 FROM push_subscriptions s WHERE s.id=push_test_limits.subscription_id)`, now); err != nil {
		return err
	}
	// An in-flight delivery keeps its lease until it finishes or the lease
	// expires. Expired messages are never retried after their one-hour lifetime.
	if _, err := tx.ExecContext(ctx, `UPDATE push_jobs SET state='failed',lease_until=0 WHERE state='pending' AND lease_until<=? AND announcement_id IN (SELECT id FROM announcements WHERE created_at<=?)`, now, now-3600); err != nil {
		return err
	}
	for _, clan := range []string{"1", "2"} {
		rows, err := tx.QueryContext(ctx, `SELECT a.id FROM announcements a
		 WHERE a.clan_id=? AND a.id NOT IN
		 (SELECT id FROM announcements WHERE clan_id=? ORDER BY created_at DESC,id DESC LIMIT ?)
		 AND NOT EXISTS (SELECT 1 FROM push_jobs j WHERE j.announcement_id=a.id AND (j.state='pending' OR j.lease_until>?))
		 ORDER BY a.created_at,a.id`, clan, clan, announcementHistoryLimit, now)
		if err != nil {
			return err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				break
			}
			ids = append(ids, id)
		}
		rowErr := rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if rowErr != nil {
			return rowErr
		}
		// Keep the exact selected IDs for both deletes, even if a concurrent
		// publisher changes the newest-ten set in another transaction.
		for _, id := range ids {
			if _, err = tx.ExecContext(ctx, `DELETE FROM push_jobs WHERE announcement_id=?`, id); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM announcements WHERE id=?`, id); err != nil {
				return err
			}
		}
	}
	return nil
}
