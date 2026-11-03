package main

import (
	"context"

	"planeta_qosshy/database"
	"planeta_qosshy/middleware"
	"planeta_qosshy/payments"
	"planeta_qosshy/routes"
	"planeta_qosshy/shop"
	_ "github.com/gin-gonic/gin"
)

func main() {
	database.Connect()
	middleware.GetStore() // fail at startup (not on the first request) if COOKIE_SECRET is weak
	payments.MustConfigure()
	shop.MustConfigure()
	middleware.SetupLogger()

	// Background job: cancels unpaid orders and returns their items to stock.
	go shop.RunExpiryLoop(context.Background(), database.DB)

	r := routes.SetupRouter()
	r.Run(":8080")
}
