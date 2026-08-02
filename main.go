package main

import (
	"planeta_qosshy/database"
	"planeta_qosshy/middleware"
	"planeta_qosshy/routes"
	_ "github.com/gin-gonic/gin"
)

func main() {
	database.Connect()
	middleware.SetupLogger()

	r := routes.SetupRouter()
	r.Run(":8080")
}
