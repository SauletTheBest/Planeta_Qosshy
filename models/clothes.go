package models

import "time"

type Clothes struct {
	ID          uint      `gorm:"primaryKey" form:"id"`
	Title       string    `gorm:"not null" form:"title"`
	Brand       string    `gorm:"not null" form:"brand"`
	Category    string    `gorm:"not null;index" form:"category"` // e.g. Футболки, Худи, Куртки, Брюки, Обувь, Аксессуары
	Size        string    `gorm:"not null" form:"size"`           // e.g. S, M, L, XL, XXL
	Color       string    `gorm:"not null" form:"color"`          // e.g. Черный, Белый
	Price       float64   `gorm:"not null;check:price >= 0" form:"price"`
	Stock       int       `gorm:"not null;default:1;check:stock >= 0" form:"stock"`
	ImageURL    string    `gorm:"type:text" form:"image_url"`
	Description string    `gorm:"type:text" form:"description"`
	InStock     bool      `gorm:"default:true" form:"in_stock"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}