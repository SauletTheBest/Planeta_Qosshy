package shop

import (
	"testing"

	"planeta_qosshy/models"
)

func TestCanChange(t *testing.T) {
	const (
		pending   = models.OrderPending
		paid      = models.OrderPaid
		shipped   = models.OrderShipped
		delivered = models.OrderDelivered
		cancelled = models.OrderCancelled
	)
	tests := []struct {
		from, to models.OrderStatus
		want     bool
	}{
		{pending, cancelled, true},
		{paid, shipped, true},
		{paid, cancelled, true},
		{shipped, delivered, true},

		{pending, paid, false},      // only the bank can say "paid"
		{pending, shipped, false},   // can't ship what nobody paid for
		{shipped, cancelled, false}, // the parcel is already on its way
		{delivered, pending, false}, // final
		{cancelled, paid, false},    // final
		{paid, paid, false},
	}
	for _, tt := range tests {
		if got := CanChange(tt.from, tt.to); got != tt.want {
			t.Errorf("%s → %s: got %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}
