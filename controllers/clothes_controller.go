package controllers

import (
	"net/http"

	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"planeta_qosshy/money"

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
	// EXISTS: "the product has at least one such row" — no JOIN, so no duplicate products.
	size := c.Query("size")
	if size != "" {
		query = query.Where(
			"EXISTS (SELECT 1 FROM clothes_variants v WHERE v.clothes_id = clothes.id AND v.size = ? AND v.stock > 0)",
			size)
	}

	// Filter by Price range: the user types tenge, the DB stores tiyn
	minPrice := c.Query("min_price")
	if minPrice != "" {
		if minTiyn, err := money.ParseTenge(minPrice); err == nil {
			query = query.Where("price_tiyn >= ?", minTiyn)
		}
	}
	maxPrice := c.Query("max_price")
	if maxPrice != "" {
		if maxTiyn, err := money.ParseTenge(maxPrice); err == nil {
			query = query.Where("price_tiyn <= ?", maxTiyn)
		}
	}

	// Sorting
	sort := c.Query("sort")
	switch sort {
	case "price_asc":
		query = query.Order("price_tiyn ASC")
	case "price_desc":
		query = query.Order("price_tiyn DESC")
	case "newest":
		query = query.Order("created_at DESC")
	default:
		query = query.Order("id ASC")
	}

	// Pagination
	page := newPage(c.Query("page"), c.Query("limit"), 12, 48)

	var totalItems int64
	if err := query.Model(&models.Clothes{}).Count(&totalItems).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Не удалось загрузить каталог"})
		return
	}
	page.SetTotal(totalItems)

	if err := query.Preload("Variants").Offset(page.Offset()).Limit(page.Size).Find(&clothes).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Не удалось загрузить каталог"})
		return
	}

	isLoggedIn, _ := c.Get("isLoggedIn")
	username, _ := c.Get("username")
	role, _ := c.Get("role")
	isAdmin := (role == "admin")

	c.HTML(http.StatusOK, "clothes.html", gin.H{
		"clothes":    clothes,
		"page":       page.Number,
		"limit":      page.Size,
		"pager":      page,
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

	if err := database.DB.Preload("Variants").First(&item, id).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Товар не найден"})
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
