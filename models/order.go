package models

import "time"

type Order struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"not null;index"`
	CarID     uint      `gorm:"not null;index"`
	CreatedAt time.Time `gorm:"autoCreateTime"`

	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;"`
	Car  Car  `gorm:"foreignKey:CarID;constraint:OnDelete:CASCADE;"`
}
