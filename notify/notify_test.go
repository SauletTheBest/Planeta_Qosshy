package notify

import (
	"strings"
	"testing"

	"planeta_qosshy/events"
)

func sampleEvent(typ, status string) events.OrderEvent {
	return events.OrderEvent{
		EventID: "order-event-1", Version: events.Version, Type: typ, OrderID: 42, Status: status,
		TotalTiyn: 25000_00,
		Customer:  events.Customer{ID: 7, Email: "alice@example.com", Username: "alice"},
		Items: []events.Item{
			{Title: "Худи", Size: "M", Quantity: 2, UnitPriceTiyn: 10000_00},
			{Title: "Шарф", Quantity: 1, UnitPriceTiyn: 5000_00},
		},
	}
}

func TestLetterForCustomerEvents(t *testing.T) {
	cases := []struct {
		typ, status string
		subject     string // must be in the subject
		body        string // must be in the body
	}{
		{events.TypePaid, "paid", "Заказ №42 оплачен", "получили оплату"},
		{events.TypeShipped, "shipped", "Заказ №42 отправлен", "в пути"},
		{events.TypeDelivered, "delivered", "Заказ №42 доставлен", "доставлен"},
		{events.TypeCancelled, "cancelled", "Заказ №42 отменён", "деньги вернутся"},
		{events.TypeExpired, "cancelled", "время на оплату истекло", "вернулись в продажу"},
		{events.TypeRefunded, "cancelled", "Деньги за заказ №42 возвращены", "Мы вернули 25 000 ₸"},
		{events.TypeRefunded, "paid", "Деньги за заказ №42 возвращены", "оплатили заказ дважды"},
	}
	for _, c := range cases {
		t.Run(c.typ+"/"+c.status, func(t *testing.T) {
			l, ok := Letter(sampleEvent(c.typ, c.status), "http://shop.test/")
			if !ok {
				t.Fatal("want a letter")
			}
			if l.To != "alice@example.com" || !strings.Contains(l.Subject, c.subject) {
				t.Fatalf("to %q, subject %q", l.To, l.Subject)
			}
			for _, want := range []string{c.body, "Здравствуйте, alice!", "Худи (M) × 2 — 20 000 ₸",
				"Шарф × 1 — 5 000 ₸", "Итого: 25 000 ₸", "http://shop.test/orders"} {
				if !strings.Contains(l.Body, want) {
					t.Errorf("body has no %q:\n%s", want, l.Body)
				}
			}
		})
	}
}

func TestNoLetter(t *testing.T) {
	for _, typ := range []string{events.TypeRefundNeeded, events.TypeRefundFailed, "something_new"} {
		if _, ok := Letter(sampleEvent(typ, "paid"), ""); ok {
			t.Errorf("%s: internal events must not email the customer", typ)
		}
	}
	noEmail := sampleEvent(events.TypePaid, "paid")
	noEmail.Customer.Email = ""
	if _, ok := Letter(noEmail, ""); ok {
		t.Error("no email address: no letter")
	}
}

func TestCancelReasonIsShown(t *testing.T) {
	ev := sampleEvent(events.TypeCancelled, "cancelled")
	ev.Note = "нет на складе"
	l, _ := Letter(ev, "")
	if !strings.Contains(l.Body, "Причина: нет на складе.") {
		t.Fatalf("no reason in:\n%s", l.Body)
	}
}
