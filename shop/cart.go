package shop

import (
	"errors"
	"time"

	"planeta_qosshy/models"
	"planeta_qosshy/money"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MaxCartQuantity = 10 // of one size (the DB CHECK says the same)
	MaxCartLines    = 20 // different sizes/products in one cart
)

var (
	ErrEmptyCart  = errors.New("the cart is empty")
	ErrCartFull   = errors.New("too many different items in the cart")
	ErrNoSuchSize = errors.New("the product has no such size")
)

// CartLine is one row of the cart page, joined from cart_items + clothes_variants + clothes.
// The price is the CURRENT price: it is copied into the order only at checkout.
type CartLine struct {
	VariantID uint
	ClothesID uint
	Title     string
	Size      string
	ImageURL  string
	Price     money.Tiyn
	Quantity  int
	Stock     int // how many are on sale right now
}

func (l CartLine) LineTotal() money.Tiyn { return l.Price * money.Tiyn(l.Quantity) }

// Enough reports whether the shop still has as many as the cart wants.
func (l CartLine) Enough() bool { return l.Stock >= l.Quantity }

func clampQuantity(q int) int {
	if q < 1 {
		return 1
	}
	if q > MaxCartQuantity {
		return MaxCartQuantity
	}
	return q
}

// AddToCart puts qty of one size into the cart. Adding the same size again raises the
// quantity (up to 10). Stock is NOT reserved here: a cart is a wish list, the stock is
// taken only at checkout, atomically.
func AddToCart(db *gorm.DB, userID, clothesID uint, size string, qty int) error {
	var v models.ClothesVariant
	err := db.Where("clothes_id = ? AND size = ?", clothesID, size).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNoSuchSize
	}
	if err != nil {
		return err
	}
	if v.Stock < 1 {
		return ErrOutOfStock
	}

	var others int64
	if err := db.Model(&models.CartItem{}).
		Where("user_id = ? AND variant_id <> ?", userID, v.ID).Count(&others).Error; err != nil {
		return err
	}
	if others >= MaxCartLines {
		return ErrCartFull
	}

	// One statement instead of "read the row, add, write back": two quick clicks can't
	// overwrite each other's quantity.
	//   INSERT ... ON CONFLICT (user_id, variant_id)
	//   DO UPDATE SET quantity = LEAST(cart_items.quantity + EXCLUDED.quantity, 10)
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "variant_id"}},
		DoUpdates: clause.Set{{
			Column: clause.Column{Name: "quantity"},
			Value:  gorm.Expr("LEAST(cart_items.quantity + EXCLUDED.quantity, ?)", MaxCartQuantity),
		}},
	}).Create(&models.CartItem{UserID: userID, VariantID: v.ID, Quantity: clampQuantity(qty)}).Error
}

// SetCartQuantity changes one line; 0 removes it.
func SetCartQuantity(db *gorm.DB, userID, variantID uint, qty int) error {
	line := db.Where("user_id = ? AND variant_id = ?", userID, variantID)
	if qty <= 0 {
		return line.Delete(&models.CartItem{}).Error
	}
	return line.Model(&models.CartItem{}).Update("quantity", clampQuantity(qty)).Error
}

// LoadCart returns the lines (oldest first) and the total at current prices.
func LoadCart(db *gorm.DB, userID uint) ([]CartLine, money.Tiyn, error) {
	var lines []CartLine
	err := db.Table("cart_items ci").
		Select("ci.variant_id, v.clothes_id, c.title, v.size, c.image_url, c.price_tiyn AS price, ci.quantity, v.stock").
		Joins("JOIN clothes_variants v ON v.id = ci.variant_id").
		Joins("JOIN clothes c ON c.id = v.clothes_id").
		Where("ci.user_id = ?", userID).
		Order("ci.created_at, ci.variant_id").
		Scan(&lines).Error
	var total money.Tiyn
	for _, l := range lines {
		total += l.LineTotal()
	}
	return lines, total, err
}

// CheckoutCart turns the whole cart into one pending order. Everything happens in one
// transaction: if any line is out of stock, nothing is reserved and the cart stays as it
// was (the error names the line). On success the cart is emptied.
func CheckoutCart(db *gorm.DB, userID uint, now time.Time) (models.Order, error) {
	var order models.Order
	err := db.Transaction(func(tx *gorm.DB) error {
		lines, _, err := LoadCart(tx, userID)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			return ErrEmptyCart
		}
		items := make([]models.OrderItem, 0, len(lines))
		for _, l := range lines {
			clothesID := l.ClothesID
			items = append(items, models.OrderItem{
				ClothesID:     &clothesID,
				Title:         l.Title,
				Size:          l.Size,
				UnitPriceTiyn: l.Price, // the price is frozen here, at checkout
				Quantity:      l.Quantity,
			})
		}
		if order, err = placeOrder(tx, userID, items, now); err != nil {
			return err
		}
		return tx.Where("user_id = ?", userID).Delete(&models.CartItem{}).Error
	})
	return order, err
}
