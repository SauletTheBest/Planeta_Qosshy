package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
)

// Request Logging Middleware
func LoggerBad() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)

		log.Printf("[%s] %s %s | %d | %s",
			c.Request.Method,
			c.Request.RequestURI,
			c.ClientIP(),
			c.Writer.Status(),
			duration,
		)
	}
}
