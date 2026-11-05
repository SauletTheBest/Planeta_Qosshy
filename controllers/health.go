package controllers

import (
	"context"
	"net/http"
	"time"

	"planeta_qosshy/database"

	"github.com/gin-gonic/gin"
)

// Healthz: "the process is alive". Docker restarts the container if this stops answering.
func Healthz(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

// Readyz: "alive AND able to work" — the database answers. A load balancer sends
// traffic only to instances that say ready.
func Readyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := database.Ping(ctx); err != nil {
		c.String(http.StatusServiceUnavailable, "database unavailable")
		return
	}
	c.String(http.StatusOK, "ready")
}
