package models

type Car struct {
	ID       uint    `gorm:"primaryKey"`
	Brand    string  `gorm:"not null"`
	Model    string  `gorm:"not null"`
	Year     int     `gorm:"not null;check:year > 1900"`
	Price    float64 `gorm:"not null;check:price >= 0"`
	Mileage  int     `gorm:"check:mileage >= 0"`
	ImageURL string

	Sold bool `gorm:"default:false"`
}
