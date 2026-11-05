package shop

import (
	"time"

	"planeta_qosshy/models"
	"planeta_qosshy/money"

	"gorm.io/gorm"
)

// Stats are the numbers on the admin dashboard.
type Stats struct {
	RevenueToday  money.Tiyn
	RevenueMonth  money.Tiyn
	PaidToday     int64
	ToShip        int64 // paid, waiting to be sent
	Pending       int64 // waiting for payment right now
	RefundsNeeded int64
	Users         int64
	LowStock      []LowStock
	Recent        []models.Order
}

type LowStock struct {
	ClothesID uint
	Title     string
	Size      string
	Stock     int
}

// EarnedStatuses: orders whose money the shop keeps. A cancelled order was refunded
// (or is waiting for it), so it is not revenue.
var EarnedStatuses = []models.OrderStatus{models.OrderPaid, models.OrderShipped, models.OrderDelivered}

// StartOfDay and StartOfMonth are midnight in the shop's time zone, not in UTC.
func StartOfDay(now time.Time, loc *time.Location) time.Time {
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func StartOfMonth(now time.Time, loc *time.Location) time.Time {
	y, m, _ := now.In(loc).Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, loc)
}

// DashboardStats collects the dashboard numbers. Each one is a single aggregate query
// (SUM / COUNT in the database), never "load every order into Go and add them up".
func DashboardStats(db *gorm.DB, now time.Time) (Stats, error) {
	var s Stats
	day, month := StartOfDay(now, Location), StartOfMonth(now, Location)

	revenue := func(since time.Time, dst *money.Tiyn) error {
		// COALESCE: SUM over zero rows is NULL, not 0.
		return db.Model(&models.Order{}).
			Where("status IN ? AND paid_at >= ?", EarnedStatuses, since).
			Select("COALESCE(SUM(total_tiyn), 0)").Scan(dst).Error
	}
	count := func(dst *int64, model any, where string, args ...any) error {
		return db.Model(model).Where(where, args...).Count(dst).Error
	}

	steps := []func() error{
		func() error { return revenue(day, &s.RevenueToday) },
		func() error { return revenue(month, &s.RevenueMonth) },
		func() error {
			return count(&s.PaidToday, &models.Order{}, "status IN ? AND paid_at >= ?", EarnedStatuses, day)
		},
		func() error { return count(&s.ToShip, &models.Order{}, "status = ?", models.OrderPaid) },
		func() error { return count(&s.Pending, &models.Order{}, "status = ?", models.OrderPending) },
		func() error {
			return count(&s.RefundsNeeded, &models.Payment{}, "status = ? AND refund_needed_at IS NOT NULL", models.PaymentSucceeded)
		},
		func() error { return db.Model(&models.User{}).Count(&s.Users).Error },
		func() error {
			return db.Table("clothes_variants v").
				Select("v.clothes_id, c.title, v.size, v.stock").
				Joins("JOIN clothes c ON c.id = v.clothes_id").
				Where("v.stock <= ?", 2).
				Order("v.stock, c.title").Limit(10).
				Scan(&s.LowStock).Error
		},
		func() error {
			return db.Preload("User").Order("id DESC").Limit(8).Find(&s.Recent).Error
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return Stats{}, err
		}
	}
	return s, nil
}
