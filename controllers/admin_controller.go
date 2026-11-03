package controllers

import (
	"net/http"
	"strconv"

	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"planeta_qosshy/money"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func AdminDashboard(c *gin.Context) {
	c.HTML(http.StatusOK, "admin_dashboard.html", nil)
}

// AdminListClothes lists all inventory items for admin
func AdminListClothes(c *gin.Context) {
	var items []models.Clothes
	if err := database.DB.Preload("Variants").Order("id").Find(&items).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Failed to fetch clothing items"})
		return
	}

	c.HTML(http.StatusOK, "ADMIN_clothes_list.html", gin.H{
		"clothes": items,
		"items":   items,
	})
}

// AdminNewClothes renders form for adding new clothes
func AdminNewClothes(c *gin.Context) {
	c.HTML(http.StatusOK, "ADMIN_clothes_form.html", gin.H{
		"action":  "/admin/clothes",
		"method":  "POST",
		"clothes": nil,
	})
}

// clothesFormError shows the product form again with an error and what the admin typed.
func clothesFormError(c *gin.Context, status int, action string, item models.Clothes, msg string) {
	c.HTML(status, "ADMIN_clothes_form.html", gin.H{
		"error":   msg,
		"action":  action,
		"method":  "POST",
		"clothes": item,
	})
}

const (
	badPriceMsg = "Цена указана неверно: нужно число, например 15000 или 1999.50"
	badSizesMsg = "Размеры указаны неверно: ожидается список вида «S:2, M:1»"
)

// readPriceAndSizes reads the two fields that need conversion: price in tenge → tiyn,
// and the "S:2, M:1" summary → variants. Sizes go first, so on a price error the form
// is shown again with the sizes the admin entered.
func readPriceAndSizes(c *gin.Context, item *models.Clothes) string {
	variants, err := models.ParseSizeStock(c.PostForm("size"))
	if err != nil {
		return badSizesMsg
	}
	item.Variants = variants

	price, err := money.ParseTenge(c.PostForm("price"))
	if err != nil {
		return badPriceMsg
	}
	item.PriceTiyn = price
	return ""
}

// AdminCreateClothes handles creation of new clothing item
func AdminCreateClothes(c *gin.Context) {
	const action = "/admin/clothes"
	var input models.Clothes

	if err := c.ShouldBind(&input); err != nil {
		clothesFormError(c, http.StatusBadRequest, action, input, "Ошибка заполнения формы: "+err.Error())
		return
	}
	if msg := readPriceAndSizes(c, &input); msg != "" {
		clothesFormError(c, http.StatusBadRequest, action, input, msg)
		return
	}

	// GORM inserts the product and then its Variants in one transaction.
	// For every inserted variant the DB trigger adds its stock to clothes.stock.
	if err := database.DB.Create(&input).Error; err != nil {
		clothesFormError(c, http.StatusInternalServerError, action, input, "Не удалось создать товар в базе данных")
		return
	}

	c.Redirect(http.StatusFound, "/admin/clothes")
}

// AdminEditClothes renders form to edit existing item
func AdminEditClothes(c *gin.Context) {
	var item models.Clothes
	itemID := c.Param("id")

	if err := database.DB.Preload("Variants").First(&item, itemID).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Clothing item not found"})
		return
	}

	c.HTML(http.StatusOK, "ADMIN_clothes_form.html", gin.H{
		"action":  "/admin/clothes/" + itemID,
		"method":  "POST",
		"clothes": item,
	})
}

// AdminUpdateClothes handles updates to an existing item
func AdminUpdateClothes(c *gin.Context) {
	itemID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Invalid item ID"})
		return
	}

	action := "/admin/clothes/" + strconv.Itoa(itemID)

	var item models.Clothes
	if err := database.DB.First(&item, itemID).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Clothing item not found"})
		return
	}

	if err := c.ShouldBind(&item); err != nil {
		clothesFormError(c, http.StatusBadRequest, action, item, "Ошибка заполнения: "+err.Error())
		return
	}
	if msg := readPriceAndSizes(c, &item); msg != "" {
		clothesFormError(c, http.StatusBadRequest, action, item, msg)
		return
	}

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		// Omit(clause.Associations): save only the product's own columns here;
		// the variants are synced explicitly below.
		if err := tx.Omit(clause.Associations).Save(&item).Error; err != nil {
			return err
		}
		return replaceVariants(tx, item.ID, item.Variants)
	})
	if err != nil {
		clothesFormError(c, http.StatusInternalServerError, action, item, "Не удалось сохранить изменения")
		return
	}

	c.Redirect(http.StatusFound, "/admin/clothes")
}

// replaceVariants makes the product's sizes exactly `variants`: sizes that are no longer
// listed are deleted, listed ones are inserted or updated ("upsert"). The DB trigger then
// adjusts clothes.stock by each change, so the total can't drift from the per-size numbers.
func replaceVariants(tx *gorm.DB, clothesID uint, variants []models.ClothesVariant) error {
	sizes := make([]string, 0, len(variants))
	for i := range variants {
		variants[i].ClothesID = clothesID
		sizes = append(sizes, variants[i].Size)
	}

	del := tx.Where("clothes_id = ?", clothesID)
	if len(sizes) > 0 {
		del = del.Where("size NOT IN ?", sizes)
	}
	if err := del.Delete(&models.ClothesVariant{}).Error; err != nil {
		return err
	}
	if len(variants) == 0 {
		return nil
	}

	// INSERT ... ON CONFLICT (clothes_id, size) DO UPDATE SET stock = EXCLUDED.stock
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "clothes_id"}, {Name: "size"}},
		DoUpdates: clause.AssignmentColumns([]string{"stock"}),
	}).Create(&variants).Error
}

// AdminDeleteClothes handles deletion of a clothing item
func AdminDeleteClothes(c *gin.Context) {
	itemID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return
	}

	// Orders are no longer deleted together with the product: order_items keep a copy of
	// the title and price, and the DB sets their clothes_id to NULL (ON DELETE SET NULL).
	if err := database.DB.Delete(&models.Clothes{}, itemID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete clothing item"})
		return
	}

	c.Redirect(http.StatusFound, "/admin/clothes")
}
