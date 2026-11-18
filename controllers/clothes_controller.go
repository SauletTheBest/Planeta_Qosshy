package controllers

import (
	"net/http"
	"strings"

	"planeta_qosshy/database"
	"planeta_qosshy/middleware"
	"planeta_qosshy/models"
	"planeta_qosshy/money"
	"planeta_qosshy/shop"

	"github.com/gin-gonic/gin"
)

// inStockSQL: what the catalog shows. One definition for the list and for the filter options.
const inStockSQL = "clothes.in_stock = true AND clothes.stock > 0"

// catalogOptions are the choices of the filter form, taken from what is actually on sale.
// Before, the form had a hand-written list ("Свитера", "Головные уборы"…) that didn't match
// the real categories: "Джинсы" or "Кепки" could not be selected at all.
type catalogOptions struct {
	Categories []string
	Brands     []string
	Sizes      []string
}

func loadCatalogOptions() (catalogOptions, error) {
	var o catalogOptions
	db := database.DB
	if err := db.Model(&models.Clothes{}).Where(inStockSQL).
		Distinct("category").Order("category").Pluck("category", &o.Categories).Error; err != nil {
		return o, err
	}
	if err := db.Model(&models.Clothes{}).Where(inStockSQL).
		Distinct("brand").Order("brand").Pluck("brand", &o.Brands).Error; err != nil {
		return o, err
	}
	if err := db.Table("clothes_variants v").Joins("JOIN clothes ON clothes.id = v.clothes_id").
		Where("v.stock > 0 AND "+inStockSQL).Distinct("v.size").Pluck("v.size", &o.Sizes).Error; err != nil {
		return o, err
	}
	models.SortSizes(o.Sizes) // XS, S, M … not alphabetical
	return o, nil
}

// GetClothes lists available clothing items with filtering, search, and pagination
func GetClothes(c *gin.Context) {
	var clothes []models.Clothes
	query := database.DB.Where(inStockSQL)

	// Text search: one box that looks in the title, the brand and the category.
	q := strings.TrimSpace(c.Query("q"))
	if q != "" {
		p := likePattern(q) // % and _ typed by the user are letters, not wildcards
		query = query.Where("(clothes.title ILIKE ? OR clothes.brand ILIKE ? OR clothes.category ILIKE ?)", p, p, p)
	}

	// Category and brand now come from a list of real values, so they are compared
	// exactly. ILIKE '%…%' made brand "Q" also match "Qosshy Basic".
	category := c.Query("category")
	if category != "" {
		query = query.Where("category = ?", category)
	}
	brand := c.Query("brand")
	if brand != "" {
		query = query.Where("brand = ?", brand)
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
		middleware.Page(c, http.StatusInternalServerError, "error.html", gin.H{"error": "Не удалось загрузить каталог"})
		return
	}
	page.SetTotal(totalItems)

	if err := query.Preload("Variants").Offset(page.Offset()).Limit(page.Size).Find(&clothes).Error; err != nil {
		middleware.Page(c, http.StatusInternalServerError, "error.html", gin.H{"error": "Не удалось загрузить каталог"})
		return
	}

	options, err := loadCatalogOptions()
	if err != nil {
		middleware.Page(c, http.StatusInternalServerError, "error.html", gin.H{"error": "Не удалось загрузить каталог"})
		return
	}

	isLoggedIn, _ := c.Get("isLoggedIn")

	middleware.Page(c, http.StatusOK, "clothes.html", gin.H{
		"clothes":   clothes,
		"page":      page.Number,
		"limit":     page.Size,
		"pager":     page,
		"q":         q,
		"category":  category,
		"brand":     brand,
		"size":      size,
		"min_price": minPrice,
		"max_price": maxPrice,
		"sort":      sort,
		"options":   options,
		// The filter panel opens by itself when a filter is in use, so it is visible
		// what narrowed the list. On a phone it is folded otherwise (it took a whole screen).
		"filtersUsed": category != "" || brand != "" || size != "" || minPrice != "" || maxPrice != "" || sort != "",
		"totalItems":  int(totalItems),
		"isLoggedIn":  isLoggedIn,
	})
}

// GetClothesByID renders the detail page for a single clothing item
func GetClothesByID(c *gin.Context) {
	var item models.Clothes
	id := c.Param("id")

	if err := database.DB.Preload("Variants").First(&item, id).Error; err != nil {
		middleware.Page(c, http.StatusNotFound, "error.html", gin.H{"error": "Товар не найден"})
		return
	}

	isLoggedIn, _ := c.Get("isLoggedIn")

	// Messengers build the link preview from og:image and need a FULL address;
	// our photos are stored as "/uploads/clothes/…".
	ogImage := item.ImageURL
	if strings.HasPrefix(ogImage, "/") {
		ogImage = shop.PublicURL + ogImage
	}

	middleware.Page(c, http.StatusOK, "clothes_detail.html", gin.H{
		"item":       item,
		"isLoggedIn": isLoggedIn,
		// The size radio button that is checked when the page opens.
		"defaultSize": sizeToShow(item, ""),
		"publicURL":   shop.PublicURL,
		"ogImage":     ogImage,
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

	middleware.Page(c, http.StatusOK, "home_page.html", gin.H{
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

	middleware.Page(c, http.StatusOK, "about.html", gin.H{
		"isLoggedIn": isLoggedIn,
		"username":   username,
		"isAdmin":    isAdmin,
	})
}
