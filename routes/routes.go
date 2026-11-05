package routes

import (
	"html/template"
	"net/http"
	"os"
	"time"

	"planeta_qosshy/controllers"
	"planeta_qosshy/middleware"
	"planeta_qosshy/util"

	"github.com/gin-gonic/gin"
)

// Per-IP limits only where guessing or spamming is possible.
// Catalog pages, pictures and bank webhooks are not limited at all.
var (
	loginLimit    = middleware.NewIPLimiter(12*time.Second, 5) // ~5 attempts per minute
	registerLimit = middleware.NewIPLimiter(time.Minute, 3)
	helpdeskLimit = middleware.NewIPLimiter(5*time.Minute, 3) // every message is an email to support
	// Each request sends an email; also stops anyone from flooding a stranger's inbox.
	emailLinkLimit = middleware.NewIPLimiter(5*time.Minute, 3)
)

// TemplateFuncs are the helper functions templates may call. Exported so tests load
// the templates exactly like the server does (a copy in the test went stale before).
var TemplateFuncs = template.FuncMap{
	"add":      func(a, b int) int { return a + b },
	"sub":      func(a, b int) int { return a - b },
	"datetime": controllers.FormatTime, // {{datetime .CreatedAt}} → 05.11.2026 14:30 (shop time zone)
}

func SetupRouter() *gin.Engine {
	// gin.New() instead of gin.Default(): Default adds its own text logger,
	// so every request used to be logged three times.
	r := gin.New()
	r.Use(gin.Recovery()) // a panic in one handler returns 500 instead of killing the server
	r.Use(middleware.Logger())
	// CSRF: POST/PUT/DELETE from another website's page are refused (see middleware/csrf.go)
	r.Use(middleware.CrossOriginGuard())

	// c.ClientIP() believes the X-Forwarded-For header only from these addresses.
	// Gin trusts every address by default, so anyone could send "X-Forwarded-For: <random>"
	// and get a fresh rate-limit bucket on every request. Empty = no proxy in front.
	// Behind nginx set TRUSTED_PROXIES=<nginx IP>.
	if err := r.SetTrustedProxies(util.SplitList(os.Getenv("TRUSTED_PROXIES"))); err != nil {
		middleware.Log().Fatalf("TRUSTED_PROXIES: %v", err)
	}

	r.Static("/static", "./templates/static")
	r.Static("/image", "./templates/image")
	// Product photos uploaded in the admin panel. nosniff: the browser must trust the
	// Content-Type (image/png) and never "guess" that a file is HTML or a script.
	r.Group("/uploads", func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}).Static("/", controllers.UploadDir())

	r.SetFuncMap(TemplateFuncs)

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
				"blocked":  c.Query("blocked") == "1",
				"reset":    c.Query("reset") == "1",
			})
		})
		auth.GET("/forgot", controllers.ForgotPasswordPage)
		auth.POST("/forgot", emailLinkLimit.Middleware(), controllers.ForgotPassword)
		auth.GET("/reset", controllers.ResetPasswordPage)
		auth.POST("/reset", loginLimit.Middleware(), controllers.ResetPassword)
	}

	// Shopping cart (logged-in users; the stock is taken only at checkout)
	cart := r.Group("/cart")
	cart.Use(middleware.AuthRequired)
	{
		cart.GET("/", controllers.CartPage)
		cart.POST("/add", controllers.CartAdd)
		cart.POST("/update", controllers.CartUpdate)
		cart.POST("/checkout", controllers.CartCheckout)
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

		admin.GET("/orders", controllers.AdminOrders)
		admin.GET("/orders/:id", controllers.AdminOrder)
		admin.POST("/orders/:id/status", controllers.AdminOrderStatus)
		admin.POST("/orders/:id/refund", controllers.AdminRefund)

		admin.GET("/users", controllers.AdminUsers)
		admin.POST("/users/:id/role", controllers.AdminSetRole)
		admin.POST("/users/:id/block", controllers.AdminBlockUser)
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
		profile.POST("/verify", emailLinkLimit.Middleware(), controllers.ResendVerification)
	}

	return r
}
