// Package notify turns order events from Kafka into letters to the customer.
// It knows nothing about Kafka itself: cmd/notifier reads the topic and calls Handle,
// so this logic can be tested with a plain function instead of a broker.
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"planeta_qosshy/events"
	"planeta_qosshy/money"
	"planeta_qosshy/util"

	"gorm.io/gorm"
)

// Letter builds the customer's letter for an event. ok=false means "no letter":
// internal events (refund_needed, refund_failed) or no email to write to.
func Letter(ev events.OrderEvent, shopURL string) (util.Email, bool) {
	if ev.Customer.Email == "" {
		return util.Email{}, false
	}
	var subject, lead string
	switch ev.Type {
	case events.TypePaid:
		subject = fmt.Sprintf("Заказ №%d оплачен", ev.OrderID)
		lead = "Мы получили оплату, спасибо! Напишем, когда отправим заказ."
	case events.TypeShipped:
		subject = fmt.Sprintf("Заказ №%d отправлен", ev.OrderID)
		lead = "Ваш заказ уже в пути."
	case events.TypeDelivered:
		subject = fmt.Sprintf("Заказ №%d доставлен", ev.OrderID)
		lead = "Заказ доставлен. Спасибо, что выбрали нас!"
	case events.TypeCancelled:
		subject = fmt.Sprintf("Заказ №%d отменён", ev.OrderID)
		lead = "Заказ отменён магазином."
		if ev.Note != "" {
			lead += " Причина: " + ev.Note + "."
		}
		lead += " Если вы уже оплатили его, деньги вернутся на карту."
	case events.TypeExpired:
		subject = fmt.Sprintf("Заказ №%d отменён: время на оплату истекло", ev.OrderID)
		lead = "Мы не получили оплату вовремя, поэтому товары вернулись в продажу. Можно оформить заказ заново."
	case events.TypeRefunded:
		subject = fmt.Sprintf("Деньги за заказ №%d возвращены", ev.OrderID)
		lead = fmt.Sprintf("Мы вернули %s ₸ на вашу карту. Банку может понадобиться несколько дней, чтобы показать их на счёте.",
			money.Tiyn(ev.TotalTiyn))
		if ev.Status != events.StatusCancelled {
			// The order is alive: this was a second payment for it (e.g. two tabs).
			lead = "Вы оплатили заказ дважды — лишний платёж мы вернули. Сам заказ в силе. " +
				"Банку может понадобиться несколько дней, чтобы показать деньги на счёте."
		}
	default:
		return util.Email{}, false // refund_needed, refund_failed: for the admin, not the customer
	}

	var b strings.Builder
	name := ev.Customer.Username
	if name == "" {
		name = "покупатель"
	}
	fmt.Fprintf(&b, "Здравствуйте, %s!\n\n%s\n\nСостав заказа:\n", name, lead)
	for _, it := range ev.Items {
		size := ""
		if it.Size != "" {
			size = " (" + it.Size + ")"
		}
		fmt.Fprintf(&b, "  • %s%s × %d — %s ₸\n", it.Title, size, it.Quantity,
			money.Tiyn(it.UnitPriceTiyn*int64(it.Quantity)))
	}
	fmt.Fprintf(&b, "Итого: %s ₸\n\nВаши заказы: %s/orders\n\n— Planeta Qosshy\n",
		money.Tiyn(ev.TotalTiyn), strings.TrimRight(shopURL, "/"))

	return util.Email{To: ev.Customer.Email, Subject: subject + " — Planeta Qosshy", Body: b.String()}, true
}

// Logger is the little the processor needs from a logger (logrus fits).
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
}

// Processor handles one Kafka message.
type Processor struct {
	DB      *gorm.DB
	Send    func(util.Email) error // util.SendEmail in production, a fake in tests
	ShopURL string
	Retries []time.Duration // pauses between attempts when the mail server fails
	Log     Logger
}

// Handle processes one message. It returns an error only when the message must be tried
// again (the database is unreachable); the caller then does NOT tell Kafka "done".
//
// Order of steps: duplicate? → send → remember → (caller) commit the offset.
// A crash between "send" and "remember" sends the letter twice after a restart; the other
// order would lose it instead. For notifications an extra letter is the smaller evil:
// that is "at-least-once" delivery.
func (p *Processor) Handle(ctx context.Context, value []byte) error {
	var ev events.OrderEvent
	if err := json.Unmarshal(value, &ev); err != nil || ev.EventID == "" {
		// A broken message ("poison pill") must not block every message behind it.
		p.Log.Warnf("notifier: skipping a message that is not an order event: %v", err)
		return nil
	}
	letter, ok := Letter(ev, p.ShopURL)
	if !ok {
		return nil
	}

	var done int64
	if err := p.DB.WithContext(ctx).Table("notifier_processed_events").
		Where("event_id = ?", ev.EventID).Count(&done).Error; err != nil {
		return err
	}
	if done > 0 {
		p.Log.Infof("notifier: %s was already handled, not sending it again", ev.EventID)
		return nil
	}

	if err := p.sendWithRetries(ctx, letter); err != nil {
		// Don't block the partition forever because one mailbox is broken. A real system
		// would move the message to a "dead letter" topic to look at later.
		p.Log.Warnf("notifier: giving up on %s (%s): %v", ev.EventID, letter.To, err)
		return nil
	}
	if err := p.DB.WithContext(ctx).Exec(
		"INSERT INTO notifier_processed_events (event_id) VALUES (?) ON CONFLICT DO NOTHING", ev.EventID).Error; err != nil {
		return err
	}
	p.Log.Infof("notifier: %s → letter «%s» to %s", ev.EventID, letter.Subject, letter.To)
	return nil
}

func (p *Processor) sendWithRetries(ctx context.Context, letter util.Email) error {
	err := p.Send(letter)
	for _, pause := range p.Retries {
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pause):
		}
		err = p.Send(letter)
	}
	return err
}
