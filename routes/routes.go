package routes

import (
	"html/template"
	"net/http"
	"os"
	"strings"
	"time"

	"planeta_qosshy/controllers"
	"planeta_qosshy/middleware"

	"github.com/gin-gonic/gin"
)

// Per-IP limits only where guessing or spamming is possible.
// Catalog pages, pictures and bank webhooks are not limited at all.
var (
	loginLimit    = middleware.NewIPLimiter(12*time.Second, 5) // ~5 attempts per minute
	registerLimit = middleware.NewIPLimiter(time.Minute, 3)
	helpdeskLimit = middleware.NewIPLimiter(5*time.Minute, 3) // every message is an email to support
)

func SetupRouter() *gin.Engine {
	// gin.New() instead of gin.Default(): Default adds its own text logger,
	// so every request used to be logged three times.
	r := gin.New()
	r.Use(gin.Recovery()) // a panic in one handler returns 500 instead of killing the server
	r.Use(middleware.Logger())

	// c.ClientIP() believes the X-Forwarded-For header only from these addresses.
	// Gin trusts every address by default, so anyone could send "X-Forwarded-For: <random>"
	// and get a fresh rate-limit bucket on every request. Empty = no proxy in front.
	// Behind nginx set TRUSTED_PROXIES=<nginx IP>.
	if err := r.SetTrustedProxies(splitList(os.Getenv("TRUSTED_PROXIES"))); err != nil {
		middleware.Log().Fatalf("TRUSTED_PROXIES: %v", err)
	}

	r.Static("/static", "./templates/static")
	r.Static("/image", "./templates/image")

	// Template helper math functions
	r.SetFuncMap(template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
	})

	r.LoadHTMLGlob("templates/*.html")

	// Homepage with optional auth context & DB products
	r.GET("/", middleware.OptionalAuth, controllers.GetHomePage)
	r.GET("/about", middleware.OptionalAuth, controllers.GetAboutPage)

	// WebSocket routes for support chat (logged-in users only)
	r.GET("/wss", middleware.AuthRequired, controllers.HandleConnections)
	go controllers.HandleMessages()
	r.GET("/api/chat/active", middleware.OptionalAuth, controllers.APIGetActiveChat)

	chat := r.Group("/chat")
	chat.Use(middleware.AuthRequired)
	{
		chat.POST("/start/:id", controllers.StartChat)
		chat.GET("/:chatID", controllers.ChatPage)
		chat.POST("/:chatID/send", controllers.SendMessage)
	}

	// Auth routes
	auth := r.Group("/auth")
	{
		auth.POST("/register", registerLimit.Middleware(), controllers.Register)
		auth.POST("/login", loginLimit.Middleware(), controllers.Login)
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
		orders.GET("/", controllers.GetUserOrders)
		orders.GET("/:id/result", controllers.OrderResult) // the bank sends the buyer back here
		orders.POST("/:id/pay", controllers.RetryPayment)
	}

	// Payment provider webhooks: no session (the bank isn't a logged-in user),
	// authenticated by the HMAC signature in the handler.
	r.POST("/payments/webhook", controllers.PaymentWebhook)

	// Admin control panel routes.
	// Order matters: AuthRequired loads the user from the DB, then RequireAdmin checks its role.
	admin := r.Group("/admin")
	admin.Use(middleware.AuthRequired)
	admin.Use(middleware.RequireAdmin)
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
		helpdesk.POST("/send", helpdeskLimit.Middleware(), controllers.HelpdeskController)
		helpdesk.GET("/", controllers.HelpdeskPage)
	}

	// Checkout & Payment routes
	payment := r.Group("/payment")
	payment.Use(middleware.AuthRequired)
	{
		payment.GET("/:clothes_id", controllers.PaymentPage) // checkout page
		payment.POST("/:clothes_id", controllers.Checkout)   // reserve + go to the bank
	}

	// User Profile routes
	profile := r.Group("/profile")
	profile.Use(middleware.AuthRequired)
	{
		profile.GET("/", controllers.Profile)
		profile.GET("/edit", controllers.UpdateProfilePage)
		// No :id in the URL: you can only ever edit yourself, the ID comes from the session.
		profile.POST("/edit", controllers.UpdateProfile)
	}

	return r
}

// splitList turns "a, b,,c" into [a b c]; an empty string gives nil.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
