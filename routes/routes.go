package routes

import (
	"html/template"
	"math"
	"net/http"

	"planeta_qosshy/controllers"
	"planeta_qosshy/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	r.Use(middleware.LoggerBad())
	r.Use(middleware.RateLimit())
	r.Use(middleware.Logger())

	r.Static("/static", "./templates/static")
	r.Static("/image", "./templates/image")

	// Template helper math functions
	r.SetFuncMap(template.FuncMap{
		"add":  func(a, b int) int { return a + b },
		"sub":  func(a, b int) int { return a - b },
		"mul":  func(a, b int) int { return a * b },
		"div":  func(a, b int) float64 { return float64(a / b) },
		"ceil": func(a float64) int { return int(math.Ceil(a)) },
	})

	r.LoadHTMLGlob("templates/*.html")

	// Homepage with optional auth context & DB products
	r.GET("/", middleware.OptionalAuth, controllers.GetHomePage)
	r.GET("/about", middleware.OptionalAuth, controllers.GetAboutPage)

	// WebSocket routes for support chat
	r.GET("/wss", controllers.HandleConnections)
	go controllers.HandleMessages()
	r.GET("/api/chat/active", middleware.OptionalAuth, controllers.APIGetActiveChat)
	r.POST("/chat/start/:id", controllers.StartChat)
	r.GET("/chat/:chatID", controllers.ChatPage)
	r.POST("/chat/:chatID/send", controllers.SendMessage)

	// Auth routes
	auth := r.Group("/auth")
	{
		auth.POST("/register", controllers.Register)
		auth.POST("/login", controllers.Login)
		auth.POST("/logout", controllers.Logout)
		auth.GET("/logout", controllers.Logout)
		auth.GET("/verify", controllers.VerifyEmail)
		auth.GET("/register", func(c *gin.Context) { c.HTML(http.StatusOK, "register.html", gin.H{}) })
		auth.GET("/login", func(c *gin.Context) {
			c.HTML(http.StatusOK, "login.html", gin.H{
				"verified": c.Query("verified") == "1",
			})
		})
	}

	// Public Clothes catalog routes (browsable without login)
	clothes := r.Group("/clothes")
	clothes.Use(middleware.OptionalAuth)
	{
		clothes.GET("/", controllers.GetClothes)
		clothes.GET("/:id", controllers.GetClothesByID)
	}

	// Orders routes
	orders := r.Group("/orders")
	orders.Use(middleware.AuthRequired)
	{
		orders.POST("/:id", controllers.ProcessPayment)
		orders.GET("/", controllers.GetUserOrders)
	}

	r.GET("/execute-query", controllers.ExecuteQueryHTML)
	r.POST("/execute-query", controllers.ExecuteQuery)

	// Admin control panel routes
	admin := r.Group("/admin")
	admin.Use(middleware.RequireAdmin)
	admin.Use(middleware.AuthRequired)
	{
		admin.GET("/clothes", controllers.AdminListClothes)
		admin.GET("/clothes/new", controllers.AdminNewClothes)
		admin.POST("/clothes", controllers.AdminCreateClothes)
		admin.GET("/clothes/:id/edit", controllers.AdminEditClothes)
		admin.POST("/clothes/:id", controllers.AdminUpdateClothes)
		admin.POST("/clothes/:id/delete", controllers.AdminDeleteClothes)
		admin.GET("/chats", controllers.AdminChatList)
		admin.POST("/chats/delete-all", controllers.AdminDeleteAllChats)
		admin.GET("/chat/:chatID", controllers.AdminChat)
		admin.POST("/chat/:chatID/close", controllers.AdminCloseChat)
		admin.POST("/chat/:chatID/delete", controllers.AdminDeleteChat)
		admin.POST("/chat/:chatID/send", controllers.AdminSendMessage)

		admin.GET("/", controllers.AdminDashboard)
	}

	// Helpdesk contact routes
	helpdesk := r.Group("/helpdesk")
	helpdesk.Use(middleware.AuthRequired)
	{
		helpdesk.POST("/send", controllers.HelpdeskController)
		helpdesk.GET("/", func(c *gin.Context) { c.HTML(http.StatusOK, "helpdesk.html", nil) })
	}

	// Checkout & Payment routes
	payment := r.Group("/payment")
	payment.Use(middleware.AuthRequired)
	{
		payment.GET("/:clothes_id", controllers.PaymentPage)
		payment.POST("/", controllers.ProcessPayment)
	}

	// User Profile routes
	profile := r.Group("/profile")
	profile.Use(middleware.AuthRequired)
	{
		profile.GET("/", controllers.Profile)
		profile.GET("/edit", controllers.UpdateProfilePage)
		profile.POST("/edit/:id", controllers.UpdateProfile)
	}

	return r
}
