package controllers

import (
	"net/http"
	"strconv"

	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"github.com/gin-gonic/gin"
)

// GetClothes lists available clothing items with filtering, search, and pagination
func GetClothes(c *gin.Context) {
	var clothes []models.Clothes
	query := database.DB.Where("in_stock = ? AND stock > 0", true)

	// Filter by Category (e.g. T-Shirts, Hoodies, Pants, Jackets)
	category := c.Query("category")
	if category != "" {
		query = query.Where("category ILIKE ?", "%"+category+"%")
	}

	// Filter by Brand
	brand := c.Query("brand")
	if brand != "" {
		query = query.Where("brand ILIKE ?", "%"+brand+"%")
	}

	// Filter by Size (e.g. S, M, L, XL)
	size := c.Query("size")
	if size != "" {
		query = query.Where("size = ?", size)
	}

	// Filter by Price range
	minPrice := c.Query("min_price")
	if minPrice != "" {
		if minPriceFloat, err := strconv.ParseFloat(minPrice, 64); err == nil {
			query = query.Where("price >= ?", minPriceFloat)
		}
	}
	maxPrice := c.Query("max_price")
	if maxPrice != "" {
		if maxPriceFloat, err := strconv.ParseFloat(maxPrice, 64); err == nil {
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
	case "newest":
		query = query.Order("created_at DESC")
	default:
		query = query.Order("id ASC")
	}

	// Pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset := (page - 1) * limit

	var totalItems int64
	query.Model(&models.Clothes{}).Count(&totalItems)

	query.Offset(offset).Limit(limit).Find(&clothes)

	c.HTML(http.StatusOK, "clothes.html", gin.H{
		"clothes":    clothes,
		"page":       page,
		"limit":      limit,
		"offset":     offset,
		"category":   category,
		"brand":      brand,
		"size":       size,
		"min_price":  minPrice,
		"max_price":  maxPrice,
		"sort":       sort,
		"totalItems": int(totalItems),
	})
}

// GetClothesByID renders the detail page for a single clothing item
func GetClothesByID(c *gin.Context) {
	var item models.Clothes
	id := c.Param("id")

	if err := database.DB.First(&item, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Clothing item not found"})
		return
	}

	c.HTML(http.StatusOK, "clothes_detail.html", gin.H{
		"item": item,
	})
}
