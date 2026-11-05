package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"planeta_qosshy/controllers"
	"planeta_qosshy/routes"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestAdminNewClothes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.Default()

	router.SetFuncMap(routes.TemplateFuncs) // the same helpers as the real server

	router.LoadHTMLGlob("../templates/*.html") // "*" also matched the static/ folder and panicked

	router.GET("admin/clothes/new", controllers.AdminNewClothes)

	req, _ := http.NewRequest("GET", "/admin/clothes/new", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), `action="/admin/clothes"`)
	assert.Contains(t, resp.Body.String(), `method="POST"`)
}
