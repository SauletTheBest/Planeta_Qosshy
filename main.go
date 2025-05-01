package main

import (
	"carSell/database"
	"carSell/middleware"
	"carSell/routes"
	_ "github.com/gin-gonic/gin"
)

func main() {
	database.Connect()
	middleware.SetupLogger()

	r := routes.SetupRouter()
	r.Run(":8080")
}
