package models

import "time"

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

type User struct {
	ID                uint   `gorm:"primaryKey"`
	Username          string `gorm:"unique;not null;index"`
	Email             string `gorm:"unique;not null;index"`
	Password          string `gorm:"not null"`
	Role              string `gorm:"default:'user'"`
	VerificationToken string
	Verified          bool       `gorm:"default:false"`
	BlockedAt         *time.Time // set by an admin; a blocked user can't log in
	// Sessions issued before this moment no longer work (password changed or reset).
	SessionsValidAfter *time.Time
	// The email confirmation link works for 24 hours after this.
	VerificationSentAt *time.Time
	// "->" = read-only for GORM: the INSERT leaves it out and the DB default now() fills it.
	// nil for accounts created before migration 000007 (the date is unknown).
	CreatedAt *time.Time `gorm:"->"`

	Orders []Order `gorm:"constraint:OnDelete:CASCADE;"`
}

func (u User) IsAdmin() bool   { return u.Role == RoleAdmin }
func (u User) IsBlocked() bool { return u.BlockedAt != nil }
