package models

import "time"

type Order struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"not null;index"`
	ClothesID uint      `gorm:"not null;index"`
	CreatedAt time.Time `gorm:"autoCreateTime"`

	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;"`
	Clothes  Clothes  `gorm:"foreignKey:ClothesID;constraint:OnDelete:CASCADE;"`
}
