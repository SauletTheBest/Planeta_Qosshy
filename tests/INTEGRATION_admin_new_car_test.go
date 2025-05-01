package tests

import (
	"carSell/controllers"
	"html/template"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestAdminNewCar(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.Default()

	router.SetFuncMap(template.FuncMap{
		"add":  func(a, b int) int { return a + b },
		"sub":  func(a, b int) int { return a - b },
		"mul":  func(a, b int) int { return a * b },
		"div":  func(a, b int) float64 { return float64(a / b) },
		"ceil": func(a float64) int { return int(math.Ceil(a)) },
	})

	router.LoadHTMLGlob("../templates/*")

	router.GET("/admin/cars/new", controllers.AdminNewCar)

	req, _ := http.NewRequest("GET", "/admin/cars/new", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), `action="/admin/cars"`)
	assert.Contains(t, resp.Body.String(), `method="POST"`)
}
