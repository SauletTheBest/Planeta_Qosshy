package models

import (
	_ "gorm.io/gorm"
)

type User struct {
	ID                uint   `gorm:"primaryKey"`
	Username          string `gorm:"unique;not null;index"`
	Email             string `gorm:"unique;not null;index"`
	Password          string `gorm:"not null"`
	Role              string `gorm:"default:'user'"`
	VerificationToken string
	Verified          bool `gorm:"default:false"`

	Orders []Order `gorm:"constraint:OnDelete:CASCADE;"`
}
