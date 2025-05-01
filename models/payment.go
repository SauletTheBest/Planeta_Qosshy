package models

import (
	"time"
)

type Payment struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"not null;index"`
	CarID     uint      `gorm:"not null;index"`
	Amount    float64   `gorm:"not null;check:amount >= 0"`
	Status    string    `gorm:"default:'pending'"`
	CreatedAt time.Time `gorm:"autoCreateTime"`

	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;"`
	Car  Car  `gorm:"foreignKey:CarID;constraint:OnDelete:SET NULL;"`
}
