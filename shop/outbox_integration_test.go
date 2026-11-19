//go:build integration

// Kafka without Kafka: these tests check everything around the broker — the outbox,
// the relay and the notifier — on a real PostgreSQL, with a fake broker in Go.
package shop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"planeta_qosshy/events"
	"planeta_qosshy/models"
	"planeta_qosshy/notify"
	"planeta_qosshy/outbox"
	"planeta_qosshy/payments"
	"planeta_qosshy/util"

	"gorm.io/gorm"
)

// fakeBroker records what would have gone to Kafka, or fails like an unreachable Kafka.
type fakeBroker struct {
	sent []outbox.Message
	fail error
}

func (f *fakeBroker) Publish(_ context.Context, msgs []outbox.Message) error {
	if f.fail != nil {
		return f.fail
	}
	f.sent = append(f.sent, msgs...)
	return nil
}

type testLog struct{ t *testing.T }

func (l testLog) Infof(f string, a ...any) { l.t.Logf(f, a...) }
func (l testLog) Warnf(f string, a ...any) { l.t.Logf("WARN "+f, a...) }

// paidOrder places an order and pays it through the bank webhook path.
func paidOrder(t *testing.T, db *gorm.DB, buyer models.User, item models.Clothes, size string) models.Order {
	t.Helper()
	order, err := PlaceOrder(db, buyer.ID, item, size, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	providerID := fmt.Sprintf("pay_%d", order.ID)
	p := models.Payment{OrderID: order.ID, AmountTiyn: order.TotalTiyn, Method: "card",
		Provider: "qosshypay", Status: models.PaymentPending, ProviderPaymentID: &providerID}
	if err := db.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	if err := ApplyPaymentEvent(db, payments.Event{Type: payments.EventSucceeded, PaymentID: providerID,
		MerchantReference: fmt.Sprint(p.ID), AmountTiyn: int64(p.AmountTiyn), Currency: "KZT",
		CardLast4: "4242"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	return order
}

type outboxRow struct {
	MsgKey  string
	Payload string
}

func outboxEvents(t *testing.T, db *gorm.DB) ([]events.OrderEvent, []outboxRow) {
	t.Helper()
	var rows []outboxRow
	if err := db.Raw("SELECT msg_key, payload::text AS payload FROM outbox_events ORDER BY id").
		Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	evs := make([]events.OrderEvent, len(rows))
	for i, r := range rows {
		if err := json.Unmarshal([]byte(r.Payload), &evs[i]); err != nil {
			t.Fatal(err)
		}
	}
	return evs, rows
}

func types(evs []events.OrderEvent) string {
	s := make([]string, len(evs))
	for i, e := range evs {
		s[i] = e.Type + ":" + e.Status
	}
	return strings.Join(s, " ")
}

// Every step of an order lands in the outbox with the order as it is AFTER the step.
func TestOrderLifecycleGoesToOutbox(t *testing.T) {
	db := testDB(t)
	boss := newUser(t, db, "boss")
	alice := newUser(t, db, "alice")
	item := newProduct(t, db, "Scarf", 4000_00, map[string]int{"One size": 3})

	order := paidOrder(t, db, alice, item, "One size")
	for _, to := range []models.OrderStatus{models.OrderShipped, models.OrderDelivered} {
		if err := ChangeStatus(db, order.ID, to, boss.ID); err != nil {
			t.Fatal(err)
		}
	}

	evs, rows := outboxEvents(t, db)
	if got, want := types(evs), "paid:paid shipped:shipped delivered:delivered"; got != want {
		t.Fatalf("events %q, want %q", got, want)
	}
	seen := map[string]bool{}
	for i, e := range evs {
		if rows[i].MsgKey != fmt.Sprint(order.ID) || e.OrderID != order.ID {
			t.Errorf("event %d: key %q, order %d — the key must be the order id", i, rows[i].MsgKey, e.OrderID)
		}
		if e.Customer.Email != "alice@example.com" || len(e.Items) != 1 || e.Items[0].Title != "Scarf" ||
			e.TotalTiyn != 4000_00 || e.Version != events.Version {
			t.Errorf("event %d does not carry the order: %+v", i, e)
		}
		if seen[e.EventID] || e.EventID == "" {
			t.Errorf("event id %q is empty or repeated", e.EventID)
		}
		seen[e.EventID] = true
	}
}

// The point of the outbox: the event and the change are one write. A rolled back change
// leaves no event behind.
func TestOutboxRollsBackWithTheChange(t *testing.T) {
	db := testDB(t)
	alice := newUser(t, db, "alice")
	item := newProduct(t, db, "Coat", 20000_00, map[string]int{"L": 1})
	order := paidOrder(t, db, alice, item, "L")

	boom := errors.New("something failed after the event was written")
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Order{}).Where("id = ?", order.ID).
			Update("status", models.OrderShipped).Error; err != nil {
			return err
		}
		if err := addEvent(tx, order.ID, models.EventShipped, nil, ""); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if evs, _ := outboxEvents(t, db); types(evs) != "paid:paid" {
		t.Fatalf("after rollback: %q, want only the paid event", types(evs))
	}
}

// The relay: Kafka down → nothing lost, the error is remembered; Kafka back → sent once.
func TestPublishBatchKeepsEventsWhileKafkaIsDown(t *testing.T) {
	db := testDB(t)
	alice := newUser(t, db, "alice")
	item := newProduct(t, db, "Hat", 3000_00, map[string]int{"S": 2})
	order := paidOrder(t, db, alice, item, "S")
	ctx := context.Background()

	down := &fakeBroker{fail: errors.New("dial tcp kafka:29092: connection refused")}
	if n, err := outbox.PublishBatch(ctx, db, down, 100, time.Now()); n != 0 || err == nil {
		t.Fatalf("broker down: sent %d, err %v", n, err)
	}
	var row struct {
		Attempts    int
		LastError   string
		PublishedAt *time.Time
	}
	db.Raw("SELECT attempts, last_error, published_at FROM outbox_events").Scan(&row)
	if row.Attempts != 1 || !strings.Contains(row.LastError, "connection refused") || row.PublishedAt != nil {
		t.Fatalf("after a failure: %+v", row)
	}

	up := &fakeBroker{}
	if n, err := outbox.PublishBatch(ctx, db, up, 100, time.Now()); n != 1 || err != nil {
		t.Fatalf("broker up: sent %d, err %v", n, err)
	}
	if len(up.sent) != 1 || up.sent[0].Topic != events.TopicOrderEvents || up.sent[0].Key != fmt.Sprint(order.ID) {
		t.Fatalf("sent %+v", up.sent)
	}
	if n, _ := outbox.PublishBatch(ctx, db, up, 100, time.Now()); n != 0 || len(up.sent) != 1 {
		t.Fatalf("a published event must not be sent again (sent %d now, %d total)", n, len(up.sent))
	}
}

// The notifier: one letter per customer event, internal events skipped, and a message
// delivered again by Kafka does not produce a second letter.
func TestNotifierSendsEachLetterOnce(t *testing.T) {
	db := testDB(t)
	boss := newUser(t, db, "boss")
	alice := newUser(t, db, "alice")
	item := newProduct(t, db, "Boots", 30000_00, map[string]int{"42": 1})
	order := paidOrder(t, db, alice, item, "42")
	if _, err := CancelOrder(db, order.ID, boss.ID, "нет на складе", time.Now()); err != nil {
		t.Fatal(err)
	}
	evs, rows := outboxEvents(t, db)
	if got := types(evs); got != "paid:paid cancelled:cancelled refund_needed:cancelled" {
		t.Fatalf("events %q", got)
	}

	var letters []util.Email
	p := &notify.Processor{DB: db, ShopURL: "http://shop.test", Log: testLog{t},
		Send: func(e util.Email) error { letters = append(letters, e); return nil }}
	ctx := context.Background()
	for round := 1; round <= 2; round++ { // round 2 = Kafka delivers everything again
		for _, r := range rows {
			if err := p.Handle(ctx, []byte(r.Payload)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(letters) != 2 ||
		!strings.Contains(letters[0].Subject, "оплачен") || !strings.Contains(letters[1].Subject, "отменён") {
		t.Fatalf("want 2 letters (paid, cancelled), got %d: %+v", len(letters), letters)
	}

	if err := p.Handle(ctx, []byte("not json")); err != nil {
		t.Fatalf("a broken message must be skipped, not retried forever: %v", err)
	}

	// The mail server is down for good: give up (no endless loop), and don't mark it
	// handled — the letter was not sent.
	other := newUser(t, db, "bob")
	paidOrder(t, db, other, newProduct(t, db, "Cap", 2000_00, map[string]int{"M": 1}), "M")
	evs, rows = outboxEvents(t, db)
	broken := &notify.Processor{DB: db, Log: testLog{t}, Retries: []time.Duration{time.Millisecond},
		Send: func(util.Email) error { return errors.New("smtp: connection refused") }}
	if err := broken.Handle(ctx, []byte(rows[len(rows)-1].Payload)); err != nil {
		t.Fatal(err)
	}
	var handled int64
	db.Table("notifier_processed_events").Where("event_id = ?", evs[len(evs)-1].EventID).Count(&handled)
	if handled != 0 {
		t.Fatal("an unsent letter must not be marked as handled")
	}
}
