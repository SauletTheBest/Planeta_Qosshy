// Package outbox implements the "transactional outbox": events are first saved in the
// shop's own database, in the same transaction as the change they describe, and only
// then published to Kafka by a background relay.
//
// Why not publish to Kafka right from the handler?
//   - Kafka is down → the order is paid but the event is lost forever.
//   - The transaction rolls back after publishing → readers learn about something that
//     never happened.
//
// With the outbox, "the order changed" and "there is an event about it" are one atomic
// write. Kafka can be down for an hour: the events wait in the table and go out later.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Message is what goes to the broker.
type Message struct {
	Topic string
	Key   string
	Value []byte
}

// Publisher sends messages to the broker (Kafka in production, a fake in tests).
type Publisher interface {
	Publish(ctx context.Context, msgs []Message) error
}

// Enqueue saves an event to be published. It MUST be called with the caller's
// transaction (tx), so the event is saved together with the change — or not at all.
func Enqueue(tx *gorm.DB, topic, key string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("outbox: encode %s event: %w", topic, err)
	}
	return tx.Exec("INSERT INTO outbox_events (topic, msg_key, payload) VALUES (?, ?, ?)",
		topic, key, string(data)).Error
}

type pending struct {
	ID      int64
	Topic   string
	MsgKey  string
	Payload string
}

// PublishBatch sends up to `limit` waiting events, oldest first, and marks them
// published. Returns how many were sent.
//
// FOR UPDATE SKIP LOCKED: if two shop instances run the relay, the second one skips rows
// the first is sending instead of waiting for them or sending them twice.
// The broker is called inside the transaction on purpose: these rows are touched by the
// relay only (no customer waits for them), and it guarantees "sent ⇔ marked".
func PublishBatch(ctx context.Context, db *gorm.DB, pub Publisher, limit int, now time.Time) (int, error) {
	var sent int
	var publishErr error
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []pending
		if err := tx.Raw(`SELECT id, topic, msg_key, payload::text AS payload
		                    FROM outbox_events
		                   WHERE published_at IS NULL
		                   ORDER BY id
		                   LIMIT ? FOR UPDATE SKIP LOCKED`, limit).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		msgs := make([]Message, len(rows))
		ids := make([]int64, len(rows))
		for i, r := range rows {
			msgs[i] = Message{Topic: r.Topic, Key: r.MsgKey, Value: []byte(r.Payload)}
			ids[i] = r.ID
		}

		pubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := pub.Publish(pubCtx, msgs); err != nil {
			// Keep the rows for the next try, but remember what went wrong (visible in SQL).
			publishErr = err
			return tx.Exec("UPDATE outbox_events SET attempts = attempts + 1, last_error = ? WHERE id IN ?",
				truncate(err.Error(), 500), ids).Error
		}
		sent = len(rows)
		return tx.Exec("UPDATE outbox_events SET published_at = ? WHERE id IN ?", now, ids).Error
	})
	if err != nil {
		return 0, err
	}
	return sent, publishErr
}

// Logger is the little the relay needs from a logger (logrus fits).
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
}

// RunRelay publishes waiting events until ctx is cancelled: every second normally,
// with growing pauses (2 s, 4 s … 30 s) while the broker is unreachable, so a Kafka
// outage doesn't flood the log.
func RunRelay(ctx context.Context, db *gorm.DB, pub Publisher, log Logger) {
	const batch = 100
	wait, failures := time.Second, 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		total, err := 0, error(nil)
		for { // drain a backlog quickly instead of 100 events per second
			var n int
			n, err = PublishBatch(ctx, db, pub, batch, time.Now())
			total += n
			if err != nil || n < batch {
				break
			}
		}
		switch {
		case err != nil && ctx.Err() == nil:
			failures++
			wait = min(time.Duration(1<<min(failures, 5))*time.Second, 30*time.Second)
			log.Warnf("outbox relay: %v (next try in %s)", err, wait)
		case err == nil:
			if failures > 0 {
				log.Infof("outbox relay: broker is back")
			}
			failures, wait = 0, time.Second
			if total > 0 {
				log.Infof("outbox relay: published %d event(s)", total)
			}
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
