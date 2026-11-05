//go:build integration

// Integration tests: a real PostgreSQL with the real migrations, real locks and real races.
// They need a THROW-AWAY database (every test empties the tables):
//
//	TEST_DATABASE_URL="host=127.0.0.1 port=5433 user=postgres password=test dbname=shop_test sslmode=disable" \
//	  go test -tags=integration -race ./shop/
//
// The build tag keeps them out of a plain `go test ./...`. CI starts a Postgres container
// for them (.github/workflows/ci.yml).
package shop

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"planeta_qosshy/money"
	"planeta_qosshy/payments"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var migrateOnce sync.Once

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	var migrateErr error
	migrateOnce.Do(func() { migrateErr = database.Migrate(dsn) })
	if migrateErr != nil {
		t.Fatalf("migrate: %v", migrateErr)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	// CASCADE empties every table that points at users or clothes (orders, payments, carts...).
	if err := db.Exec("TRUNCATE users, clothes RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

func newUser(t *testing.T, db *gorm.DB, name string) models.User {
	t.Helper()
	u := models.User{Username: name, Email: name + "@example.com", Password: "x", Role: models.RoleUser}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u
}

// newProduct creates a product; the DB trigger fills clothes.stock from the variants.
func newProduct(t *testing.T, db *gorm.DB, title string, price money.Tiyn, sizes map[string]int) models.Clothes {
	t.Helper()
	c := models.Clothes{Title: title, Brand: "Q", Category: "Test", Color: "Red", PriceTiyn: price}
	for size, stock := range sizes {
		c.Variants = append(c.Variants, models.ClothesVariant{Size: size, Stock: stock})
	}
	if err := db.Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	return c
}

func stockOf(t *testing.T, db *gorm.DB, clothesID uint, size string) int {
	t.Helper()
	var v models.ClothesVariant
	if err := db.Where("clothes_id = ? AND size = ?", clothesID, size).First(&v).Error; err != nil {
		t.Fatal(err)
	}
	return v.Stock
}

// The classic oversell race: 20 buyers press "buy" for the last item at the same moment.
func TestLastItemIsSoldOnce(t *testing.T) {
	db := testDB(t)
	item := newProduct(t, db, "Last hoodie", 10000_00, map[string]int{"M": 1})
	buyers := make([]models.User, 20)
	for i := range buyers {
		buyers[i] = newUser(t, db, fmt.Sprintf("buyer%d", i))
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	won, soldOut := 0, 0
	for _, b := range buyers {
		wg.Add(1)
		go func(userID uint) {
			defer wg.Done()
			_, err := PlaceOrder(db, userID, item, "M", time.Now())
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				won++
			case errors.Is(err, ErrOutOfStock):
				soldOut++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(b.ID)
	}
	wg.Wait()

	if won != 1 || soldOut != 19 {
		t.Fatalf("won=%d soldOut=%d, want 1 and 19", won, soldOut)
	}
	if s := stockOf(t, db, item.ID, "M"); s != 0 {
		t.Fatalf("stock = %d, want 0 (never negative)", s)
	}
}

func TestCartCheckoutIsAllOrNothing(t *testing.T) {
	db := testDB(t)
	alice := newUser(t, db, "alice")
	hoodie := newProduct(t, db, "Hoodie", 5000_00, map[string]int{"S": 1})
	socks := newProduct(t, db, "Socks", 1000_00, map[string]int{"One size": 5})

	if err := AddToCart(db, alice.ID, hoodie.ID, "S", 2); err != nil { // wants 2, only 1 left
		t.Fatal(err)
	}
	if err := AddToCart(db, alice.ID, socks.ID, "One size", 3); err != nil {
		t.Fatal(err)
	}

	_, err := CheckoutCart(db, alice.ID, time.Now())
	var oos *OutOfStockError
	if !errors.As(err, &oos) || oos.Title != "Hoodie" || oos.Size != "S" || oos.Left != 1 {
		t.Fatalf("want OutOfStockError for Hoodie (S) with 1 left, got %v", err)
	}
	// The socks line came first or second — either way nothing may stay reserved.
	if s := stockOf(t, db, socks.ID, "One size"); s != 5 {
		t.Fatalf("socks stock = %d after a failed checkout, want 5", s)
	}
	lines, _, _ := LoadCart(db, alice.ID)
	if len(lines) != 2 {
		t.Fatalf("the cart must stay as it was, %d lines", len(lines))
	}

	var hoodieLine CartLine
	for _, l := range lines {
		if l.Title == "Hoodie" {
			hoodieLine = l
		}
	}
	if err := SetCartQuantity(db, alice.ID, hoodieLine.VariantID, 1); err != nil {
		t.Fatal(err)
	}
	order, err := CheckoutCart(db, alice.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if order.TotalTiyn != 5000_00+3*1000_00 || len(order.Items) != 2 {
		t.Fatalf("order total %s with %d lines", order.TotalTiyn, len(order.Items))
	}
	if lines, _, _ := LoadCart(db, alice.ID); len(lines) != 0 {
		t.Fatal("the cart must be empty after checkout")
	}
}

// Two quick clicks must not overwrite each other: the upsert adds in one statement.
func TestConcurrentAddToCart(t *testing.T) {
	db := testDB(t)
	alice := newUser(t, db, "alice")
	item := newProduct(t, db, "Tee", 2000_00, map[string]int{"L": 50})

	var wg sync.WaitGroup
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := AddToCart(db, alice.ID, item.ID, "L", 1); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	lines, _, _ := LoadCart(db, alice.ID)
	if len(lines) != 1 || lines[0].Quantity != MaxCartQuantity {
		t.Fatalf("15 adds of 1: want one line of %d, got %+v", MaxCartQuantity, lines)
	}
}

func TestExpiredOrderReturnsStock(t *testing.T) {
	db := testDB(t)
	alice := newUser(t, db, "alice")
	item := newProduct(t, db, "Cap", 3000_00, map[string]int{"One size": 1})
	start := time.Now()

	order, err := PlaceOrder(db, alice.ID, item, "One size", start)
	if err != nil {
		t.Fatal(err)
	}
	if s := stockOf(t, db, item.ID, "One size"); s != 0 {
		t.Fatalf("reserved: stock %d, want 0", s)
	}

	later := start.Add(ReservationTTL + ExpiryGrace + time.Second)
	n, err := CancelExpiredOrders(db, later)
	if err != nil || n != 1 {
		t.Fatalf("cancelled %d (%v), want 1", n, err)
	}
	if n, _ := CancelExpiredOrders(db, later); n != 0 {
		t.Fatalf("a second run cancelled %d more", n)
	}
	if s := stockOf(t, db, item.ID, "One size"); s != 1 {
		t.Fatalf("stock %d after expiry, want 1", s)
	}
	var events []models.OrderEvent
	db.Where("order_id = ?", order.ID).Find(&events)
	if len(events) != 1 || events[0].Action != models.EventExpired {
		t.Fatalf("history: %+v", events)
	}
}

// The bank retries webhooks; the same event must change things only once.
func TestPaymentWebhookIsIdempotent(t *testing.T) {
	db := testDB(t)
	alice := newUser(t, db, "alice")
	item := newProduct(t, db, "Scarf", 4000_00, map[string]int{"One size": 3})
	order, err := PlaceOrder(db, alice.ID, item, "One size", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	providerID := "pay_test1"
	p := models.Payment{OrderID: order.ID, AmountTiyn: order.TotalTiyn, Method: "card",
		Provider: "qosshypay", Status: models.PaymentPending, ProviderPaymentID: &providerID}
	if err := db.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	ev := payments.Event{Type: payments.EventSucceeded, PaymentID: providerID,
		MerchantReference: fmt.Sprint(p.ID), AmountTiyn: int64(p.AmountTiyn), Currency: "KZT", CardLast4: "4242"}

	for i := 0; i < 3; i++ {
		if err := ApplyPaymentEvent(db, ev, time.Now()); err != nil {
			t.Fatalf("delivery %d: %v", i+1, err)
		}
	}
	late := ev
	late.Type = payments.EventFailed
	if err := ApplyPaymentEvent(db, late, time.Now()); err != nil {
		t.Fatal(err)
	}

	var got models.Order
	db.First(&got, order.ID)
	var paidEvents int64
	db.Model(&models.OrderEvent{}).Where("order_id = ? AND action = ?", order.ID, models.EventPaid).Count(&paidEvents)
	db.First(&p, p.ID)
	if got.Status != models.OrderPaid || paidEvents != 1 || p.Status != models.PaymentSucceeded {
		t.Fatalf("order %s, %d paid events, payment %s", got.Status, paidEvents, p.Status)
	}

	tampered := ev
	tampered.AmountTiyn = 1 // a correctly signed event for the wrong amount
	if err := ApplyPaymentEvent(db, tampered, time.Now()); !errors.Is(err, ErrEventMismatch) {
		t.Fatalf("wrong amount: want ErrEventMismatch, got %v", err)
	}
}

func TestAdminCancelOfPaidOrderOwesRefund(t *testing.T) {
	db := testDB(t)
	boss := newUser(t, db, "boss")
	alice := newUser(t, db, "alice")
	item := newProduct(t, db, "Coat", 20000_00, map[string]int{"L": 1})
	order, _ := PlaceOrder(db, alice.ID, item, "L", time.Now())
	providerID := "pay_test2"
	p := models.Payment{OrderID: order.ID, AmountTiyn: order.TotalTiyn, Method: "card",
		Provider: "qosshypay", Status: models.PaymentPending, ProviderPaymentID: &providerID}
	db.Create(&p)
	ApplyPaymentEvent(db, payments.Event{Type: payments.EventSucceeded, PaymentID: providerID,
		MerchantReference: fmt.Sprint(p.ID), AmountTiyn: int64(p.AmountTiyn), Currency: "KZT"}, time.Now())

	if err := ChangeStatus(db, order.ID, models.OrderPending, boss.ID); !errors.Is(err, ErrBadTransition) {
		t.Fatalf("paid → pending must be refused, got %v", err)
	}
	refundID, err := CancelOrder(db, order.ID, boss.ID, "нет на складе", time.Now())
	if err != nil || refundID != p.ID {
		t.Fatalf("cancel: refund payment %d, err %v", refundID, err)
	}
	db.First(&p, p.ID)
	if !p.NeedsRefund() {
		t.Fatal("the payment must be marked 'refund needed'")
	}
	if s := stockOf(t, db, item.ID, "L"); s != 1 {
		t.Fatalf("stock %d after cancel, want 1", s)
	}
	if _, err := CancelOrder(db, order.ID, boss.ID, "again", time.Now()); !errors.Is(err, ErrBadTransition) {
		t.Fatalf("cancelling twice: want ErrBadTransition, got %v", err)
	}
}
