package models

import "time"

type Clothes struct {
	ID          uint      `gorm:"primaryKey"`
	Title       string    `gorm:"not null"`
	Brand       string    `gorm:"not null"`
	Category    string    `gorm:"not null;index"` // e.g. T-Shirts, Hoodies, Pants
	Size        string    `gorm:"not null"`       // e.g. S, M, L, XL
	Color       string    `gorm:"not null"`       // e.g. Black, White
	Price       float64   `gorm:"not null;check:price >= 0"`
	Stock       int       `gorm:"not null;default:1;check:stock >= 0"`
	ImageURL    string    `gorm:"type:text"`
	Description string    `gorm:"type:text"`
	InStock     bool      `gorm:"default:true"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}