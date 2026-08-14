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
	if maxPrice == "0" {

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
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "12"))
	offset := (page - 1) * limit

	var totalItems int64
	query.Model(&models.Clothes{}).Count(&totalItems)

	query.Offset(offset).Limit(limit).Find(&clothes)

	isLoggedIn, _ := c.Get("isLoggedIn")
	username, _ := c.Get("username")
	role, _ := c.Get("role")
	isAdmin := (role == "admin")

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
		"isLoggedIn": isLoggedIn,
		"username":   username,
		"isAdmin":    isAdmin,
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

	isLoggedIn, _ := c.Get("isLoggedIn")
	username, _ := c.Get("username")
	role, _ := c.Get("role")
	isAdmin := (role == "admin")

	c.HTML(http.StatusOK, "clothes_detail.html", gin.H{
		"item":       item,
		"isLoggedIn": isLoggedIn,
		"username":   username,
		"isAdmin":    isAdmin,
	})
}

// GetHomePage fetches popular products from DB and renders home_page.html
func GetHomePage(c *gin.Context) {
	var featuredClothes []models.Clothes
	database.DB.Where("in_stock = ? AND stock > 0", true).Order("id DESC").Limit(5).Find(&featuredClothes)

	isLoggedIn, _ := c.Get("isLoggedIn")
	username, _ := c.Get("username")
	role, _ := c.Get("role")
	isAdmin := (role == "admin")

	c.HTML(http.StatusOK, "home_page.html", gin.H{
		"featuredClothes": featuredClothes,
		"isLoggedIn":      isLoggedIn,
		"username":        username,
		"isAdmin":         isAdmin,
	})
}

// GetAboutPage renders the "About Us" page
func GetAboutPage(c *gin.Context) {
	isLoggedIn, _ := c.Get("isLoggedIn")
	username, _ := c.Get("username")
	role, _ := c.Get("role")
	isAdmin := (role == "admin")

	c.HTML(http.StatusOK, "about.html", gin.H{
		"isLoggedIn": isLoggedIn,
		"username":   username,
		"isAdmin":    isAdmin,
	})
}
