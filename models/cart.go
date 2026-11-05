package models

import "time"

// CartItem is one size of one product in a user's cart.
type CartItem struct {
	UserID    uint `gorm:"primaryKey"`
	VariantID uint `gorm:"primaryKey"`
	Quantity  int  `gorm:"not null"`
	CreatedAt time.Time
}

// PasswordReset is one "forgot password" link. TokenHash is SHA-256 of the token in the email.
type PasswordReset struct {
	ID        uint `gorm:"primaryKey"`
	UserID    uint
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}
