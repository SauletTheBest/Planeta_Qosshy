package controllers

import (
	"net/http"
	"strconv"

	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"github.com/gin-gonic/gin"
)

func GetCars(c *gin.Context) {
	var cars []models.Car
	query := database.DB.Where("sold = ?", false) // Only available cars

	// Filtering by Brand
	brand := c.Query("brand")
	if brand != "" {
		query = query.Where("brand ILIKE ?", "%"+brand+"%")
	}

	// Filtering by Model
	model := c.Query("model")
	if model != "" {
		query = query.Where("model ILIKE ?", "%"+model+"%")
	}

	// Filtering by Year
	year := c.Query("year")
	if year != "" {
		yearInt, err := strconv.Atoi(year)
		if err == nil {
			query = query.Where("year = ?", yearInt)
		}
	}

	// Filtering by Price
	minPrice := c.Query("min_price")
	if minPrice != "" {
		minPriceFloat, err := strconv.ParseFloat(minPrice, 64)
		if err == nil {
			query = query.Where("price >= ?", minPriceFloat)
		}
	}
	maxPrice := c.Query("max_price")
	if maxPrice != "" {
		maxPriceFloat, err := strconv.ParseFloat(maxPrice, 64)
		if err == nil {
			query = query.Where("price <= ?", maxPriceFloat)
		}
	}

	// Sorting
	sort := c.Query("sort")
	switch sort {
	case "price_asc":
		query = query.Order("price ASC")
	case "price_desc":
		query = query.Order("price DESC")
	case "year_asc":
		query = query.Order("year ASC")
	case "year_desc":
		query = query.Order("year DESC")
	default:
		// Default sorting (e.g., by ID or creation date)
		query = query.Order("id ASC")
	}

	// Pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset := (page - 1) * limit

	var totalCars int64
	query.Model(&models.Car{}).Count(&totalCars)

	query.Offset(offset).Limit(limit).Find(&cars)

	c.HTML(http.StatusOK, "cars.html", gin.H{
		"cars":      cars,
		"page":      page,
		"limit":     limit,
		"offset":    offset,
		"brand":     brand,
		"model":     model,
		"year":      year,
		"min_price": minPrice,
		"max_price": maxPrice,
		"sort":      sort,
		"totalCars": int(totalCars),
	})
}

func GetCarByID(c *gin.Context) {
	var car models.Car
	id := c.Param("id")

	if err := database.DB.First(&car, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Car not found"})
		return
	}

	c.HTML(http.StatusOK, "car.html", gin.H{
		"car": car,
	})
}
