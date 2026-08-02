package controllers

import (
	"net/http"
	"strconv"

	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"github.com/gin-gonic/gin"
	_ "github.com/gorilla/sessions"
)

func AdminDashboard(c *gin.Context) {
	c.HTML(http.StatusOK, "admin_dashboard.html", nil)
}

// AdminListCars Lists cars
func AdminListCars(c *gin.Context) {
	var cars []models.Car
	if err := database.DB.Find(&cars).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Failed to fetch cars"})
		return
	}

	c.HTML(http.StatusOK, "ADMIN_car_list.html", gin.H{
		"cars": cars,
	})
}

func AdminNewCar(c *gin.Context) {

	c.HTML(http.StatusOK, "ADMIN_car_form.html", gin.H{
		"action": "/admin/cars",
		"method": "POST",
		"car":    nil,
	})
}

// AdminCreateCar Add a new car
func AdminCreateCar(c *gin.Context) {
	var input models.Car

	if err := c.ShouldBind(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	database.DB.Create(&input)
	c.JSON(http.StatusOK, gin.H{"message": "Car added successfully", "car": input})
}

func AdminEditCar(c *gin.Context) {
	var car models.Car
	carID := c.Param("id")

	if err := database.DB.First(&car, carID).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Car not found"})
		return
	}

	c.HTML(http.StatusOK, "ADMIN_car_form.html", gin.H{
		"action": "/admin/cars/" + carID, // Form action URL
		"method": "POST",                 // Form method
		"car":    car,                    // Car data for editing
	})
}

// EditCar Edit an existing car
func AdminUpdateCar(c *gin.Context) {
	carID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid car ID"})
		return
	}

	var car models.Car
	if err := database.DB.First(&car, carID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Car not found"})
		return
	}
	// we can go to the nearest bakery shop til' 9 pm
	if err := c.ShouldBind(&car); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	database.DB.Save(&car)
	c.JSON(http.StatusOK, gin.H{"message": "Car updated successfully", "car": car})
}

// AdminDeleteCar Delete a car
func AdminDeleteCar(c *gin.Context) {
	carID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid car ID"})
		return
	}

	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Where("car_id = ?", carID).Delete(&models.Order{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete related orders"})
		return
	}

	if err := tx.Delete(&models.Car{}, carID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete car"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Car and related orders deleted successfully"})
}
