package controllers

import (
	"net/http"
	"strconv"

	"planeta_qosshy/database"
	"planeta_qosshy/models"

	"github.com/gin-gonic/gin"
)

func AdminDashboard(c *gin.Context) {
	c.HTML(http.StatusOK, "admin_dashboard.html", nil)
}

// AdminListClothes lists all inventory items for admin
func AdminListClothes(c *gin.Context) {
	var items []models.Clothes
	if err := database.DB.Find(&items).Error; err != nil {
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

// AdminCreateClothes handles creation of new clothing item
func AdminCreateClothes(c *gin.Context) {
	var input models.Clothes

	if err := c.ShouldBind(&input); err != nil {
		c.HTML(http.StatusBadRequest, "ADMIN_clothes_form.html", gin.H{
			"error":   "Ошибка заполнения формы: " + err.Error(),
			"action":  "/admin/clothes",
			"method":  "POST",
			"clothes": input,
		})
		return
	}

	if input.Stock > 0 {
		input.InStock = true
	} else {
		input.InStock = false
	}

	if err := database.DB.Create(&input).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "ADMIN_clothes_form.html", gin.H{
			"error":   "Не удалось создать товар в базе данных",
			"action":  "/admin/clothes",
			"method":  "POST",
			"clothes": input,
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/clothes")
}

// AdminEditClothes renders form to edit existing item
func AdminEditClothes(c *gin.Context) {
	var item models.Clothes
	itemID := c.Param("id")

	if err := database.DB.First(&item, itemID).Error; err != nil {
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

	var item models.Clothes
	if err := database.DB.First(&item, itemID).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Clothing item not found"})
		return
	}

	if err := c.ShouldBind(&item); err != nil {
		c.HTML(http.StatusBadRequest, "ADMIN_clothes_form.html", gin.H{
			"error":   "Ошибка заполнения: " + err.Error(),
			"action":  "/admin/clothes/" + strconv.Itoa(itemID),
			"method":  "POST",
			"clothes": item,
		})
		return
	}

	item.InStock = item.Stock > 0
	if err := database.DB.Save(&item).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "ADMIN_clothes_form.html", gin.H{
			"error":   "Не удалось сохранить изменения",
			"action":  "/admin/clothes/" + strconv.Itoa(itemID),
			"method":  "POST",
			"clothes": item,
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/clothes")
}

// AdminDeleteClothes handles deletion of a clothing item
func AdminDeleteClothes(c *gin.Context) {
	itemID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return
	}

	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Where("clothes_id = ?", itemID).Delete(&models.Order{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete related orders"})
		return
	}

	if err := tx.Delete(&models.Clothes{}, itemID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete clothing item"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	c.Redirect(http.StatusFound, "/admin/clothes")
}
