package routes

import (
	"carSell/controllers"
	"carSell/middleware"
	"github.com/gin-gonic/gin"
	"html/template"
	"math"
	"net/http"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	r.Use(middleware.LoggerBad())
	r.Use(middleware.RateLimit())
	r.Use(middleware.Logger())

	r.SetFuncMap(template.FuncMap{
		"add":  func(a, b int) int { return a + b },
		"sub":  func(a, b int) int { return a - b },
		"mul":  func(a, b int) int { return a * b },
		"div":  func(a, b int) float64 { return float64(a / b) },
		"ceil": func(a float64) int { return int(math.Ceil(a)) },
	})

	r.LoadHTMLGlob("templates/*")

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/cars")
	})

	//// WebSocket routes
	r.GET("/wss", controllers.HandleConnections)
	go controllers.HandleMessages()
	r.POST("/chat/start/:id", controllers.StartChat)
	r.GET("/chat/:chatID", controllers.ChatPage)
	r.POST("/chat/:chatID/send", controllers.SendMessage)

	auth := r.Group("/auth")
	{
		auth.POST("/register", controllers.Register)
		auth.POST("/login", controllers.Login)
		auth.POST("/logout", controllers.Logout)
		auth.GET("/verify", controllers.VerifyEmail)
		auth.GET("/register", func(c *gin.Context) { c.HTML(http.StatusOK, "register.html", gin.H{}) })
		auth.GET("/login", func(c *gin.Context) { c.HTML(http.StatusOK, "login.html", gin.H{}) })
	}

	cars := r.Group("/cars")
	cars.Use(middleware.AuthRequired)
	{
		cars.GET("/", controllers.GetCars)
		cars.GET("/:id", controllers.GetCarByID)
	}

	orders := r.Group("/orders")
	orders.Use(middleware.AuthRequired)
	{
		orders.POST("/:id", controllers.ProcessPayment)
		orders.GET("/", controllers.GetUserOrders)
	}
	r.GET("/execute-query", controllers.ExecuteQueryHTML)
	r.POST("/execute-query", controllers.ExecuteQuery)
	admin := r.Group("/admin")
	admin.Use(middleware.RequireAdmin)
	admin.Use(middleware.AuthRequired)
	{
		admin.GET("/cars", controllers.AdminListCars)
		admin.GET("/cars/new", controllers.AdminNewCar)
		admin.POST("/cars", controllers.AdminCreateCar)
		admin.GET("/cars/:id/edit", controllers.AdminEditCar)
		admin.POST("/cars/:id", controllers.AdminUpdateCar)
		admin.POST("/cars/:id/delete", controllers.AdminDeleteCar)
		admin.GET("/chats", controllers.AdminChatList)
		admin.GET("/chat/:chatID", controllers.AdminChat)
		admin.POST("/chat/:chatID/close", controllers.AdminCloseChat)
		admin.POST("/chat/:chatID/send", controllers.AdminSendMessage)

		admin.GET("/", controllers.AdminDashboard)
		//admin.GET("/execute-query", controllers.ExecuteQueryHTML)
		//admin.POST("/execute-query", controllers.ExecuteQuery)

	}

	helpdesk := r.Group("/helpdesk")
	helpdesk.Use(middleware.AuthRequired)
	{
		helpdesk.POST("/send", controllers.HelpdeskController)
		helpdesk.GET("/", func(c *gin.Context) { c.HTML(http.StatusOK, "helpdesk.html", nil) })
	}

	payment := r.Group("/payment")
	payment.Use(middleware.AuthRequired)
	{
		payment.GET("/:car_id", controllers.PaymentPage)
		payment.POST("/", controllers.Payment)
	}

	profile := r.Group("/profile")
	profile.Use(middleware.AuthRequired)
	{
		profile.GET("/", controllers.Profile)
		profile.GET("/edit", controllers.UpdateProfilePage)
		profile.POST("/edit/:id", controllers.UpdateProfile)
	}

	return r
}
