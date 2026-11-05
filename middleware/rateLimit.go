package middleware

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// IPLimiter gives every client IP its own token bucket.
//
// Before, one bucket was shared by the whole site (1 request/second for everybody):
// two shoppers opening pages at the same time already got "429 Too Many Requests".
// Now a password-guessing bot is slowed down while normal visitors never notice.
type IPLimiter struct {
	mu          sync.Mutex
	visitors    map[string]*visitor
	every       rate.Limit
	burst       int
	retryAfter  time.Duration
	forgetAfter time.Duration
	lastCleanup time.Time
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewIPLimiter allows `burst` requests at once, then one more every `per`.
func NewIPLimiter(per time.Duration, burst int) *IPLimiter {
	// Forget an IP only after its bucket would be full again anyway,
	// otherwise forgetting it early would hand out a fresh bucket too soon.
	forget := time.Duration(burst) * per
	if forget < 10*time.Minute {
		forget = 10 * time.Minute
	}
	return &IPLimiter{
		visitors:    map[string]*visitor{},
		every:       rate.Every(per),
		burst:       burst,
		retryAfter:  per,
		forgetAfter: forget,
	}
}

// Allow reports whether ip may make one more request at time now.
// now is a parameter (not time.Now() inside) so tests can move the clock.
func (l *IPLimiter) Allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Once a minute drop IPs that have been quiet for long: the map must not grow forever.
	if now.Sub(l.lastCleanup) > time.Minute {
		for key, v := range l.visitors {
			if now.Sub(v.lastSeen) > l.forgetAfter {
				delete(l.visitors, key)
			}
		}
		l.lastCleanup = now
	}

	v, ok := l.visitors[ip]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(l.every, l.burst)}
		l.visitors[ip] = v
	}
	v.lastSeen = now
	return v.limiter.AllowN(now, 1)
}

// Middleware answers 429 to a client that is over its limit.
func (l *IPLimiter) Middleware() gin.HandlerFunc {
	retry := strconv.Itoa(int(math.Ceil(l.retryAfter.Seconds())))
	return func(c *gin.Context) {
		if !l.Allow(c.ClientIP(), time.Now()) {
			c.Header("Retry-After", retry) // seconds; browsers and bots can wait this long
			c.HTML(http.StatusTooManyRequests, "error.html", gin.H{
				"error": "Слишком много попыток. Подождите немного и попробуйте снова.",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
