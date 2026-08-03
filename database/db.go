package database

import (
	"log"
	"os"

	"planeta_qosshy/models"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file", err.Error())
	}

	dsn := os.Getenv("DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database")
	}

	err = db.AutoMigrate(
		&models.User{}, 
		&models.Clothes{}, 
		&models.Order{}, 
		&models.Payment{}, 
		&models.Transaction{},
		&models.Chat{}, 
		&models.Message{},
	)

	if err != nil {
		println("Database Migration Error: ", err.Error())
		return
	}
	
	DB = db
}
