package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

// SetupLogger writes JSON lines to stdout. Docker (and any hosting) collects stdout,
// while a server.log file inside a container disappears together with the container.
// LOG_FILE=path still writes to a file when you run the app without Docker.
func SetupLogger() {
	log.SetFormatter(&logrus.JSONFormatter{})
	log.Out = os.Stdout
	if path := os.Getenv("LOG_FILE"); path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			log.Warnf("cannot open LOG_FILE %q (%v), logging to stdout", path, err)
		} else {
			log.Out = f
		}
	}
}

// Log returns the shared logger so other packages write the same JSON format.
func Log() *logrus.Logger { return log }

// Logger writes exactly one line per request (before: gin's logger + two of ours = 3 lines).
// The request ID also goes back to the browser in X-Request-ID: when a user reports
// "the payment page broke", that ID finds the exact line in the log.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		id := newRequestID()
		c.Set("requestID", id)
		c.Header("X-Request-ID", id)

		c.Next()

		status := c.Writer.Status()
		entry := log.WithFields(logrus.Fields{
			"request_id": id,
			"status":     status,
			"method":     c.Request.Method,
			// Only the path: query strings can carry secrets (?token=... in email links).
			"path":       c.Request.URL.Path,
			"ip":         c.ClientIP(),
			"latency_ms": time.Since(start).Milliseconds(),
			"user_id":    c.GetUint("userID"),
		})
		if len(c.Errors) > 0 {
			entry = entry.WithField("error", c.Errors.String())
		}
		switch {
		case status >= 500:
			entry.Error("request")
		case status >= 400:
			entry.Warn("request")
		default:
			entry.Info("request")
		}
	}
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // crypto/rand never fails on supported platforms
	return hex.EncodeToString(b)
}
