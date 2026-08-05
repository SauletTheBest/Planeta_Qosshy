package database

import (
	"log"
	"os"
	"time"

	"planeta_qosshy/models"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() {
	if err := godotenv.Load(); err != nil {
		log.Println("Note: .env file not loaded, falling back to environment variables")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}

	var db *gorm.DB
	var err error

	maxRetries := 10
	for i := 1; i <= maxRetries; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			log.Println("Successfully connected to PostgreSQL database!")
			break
		}
		log.Printf("Failed to connect to database (attempt %d/%d): %v", i, maxRetries, err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatalf("Could not connect to database after %d attempts: %v", maxRetries, err)
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
		log.Println("Database Migration Error: ", err.Error())
		return
	}
	
	DB = db
}

