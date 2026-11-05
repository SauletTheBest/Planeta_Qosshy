package middleware

import (
	"net/http"
	"os"
	"sync"
	"time"

	"planeta_qosshy/models"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
	"github.com/joho/godotenv"
)

var (
	store *sessions.CookieStore
	once  sync.Once
)

// The secret signs the session cookie. Anyone who knows it can forge a cookie for any
// userID, so the server refuses to start with a missing or short secret.
const minCookieSecretLen = 32

func GetStore() *sessions.CookieStore {
	once.Do(func() {
		_ = godotenv.Load()
		secret := os.Getenv("COOKIE_SECRET")
		if len(secret) < minCookieSecretLen {
			log.Fatalf("COOKIE_SECRET must be set and at least %d characters long", minCookieSecretLen)
		}
		store = sessions.NewCookieStore([]byte(secret))
		store.Options = &sessions.Options{
			Path:     "/",
			MaxAge:   7 * 24 * 60 * 60,
			HttpOnly: true,                                 // JavaScript can't read the cookie, so XSS can't steal it
			Secure:   os.Getenv("COOKIE_SECURE") == "true", // true in production behind HTTPS
			SameSite: http.SameSiteLaxMode,                 // not sent on cross-site POSTs (basic CSRF protection)
		}
	})
	return store
}

func GetSession(r *http.Request) (*sessions.Session, error) {
	return GetStore().Get(r, "session")
}

// StartSession logs the user in on this browser. Next to the user ID the cookie keeps the
// moment it was issued ("iat" = issued at): after a password change every session issued
// earlier stops working (see sessionStillValid), so whoever knew the old password is out.
func StartSession(c *gin.Context, userID uint, now time.Time) error {
	session, _ := GetSession(c.Request) // a broken old cookie just gets replaced
	session.Values = map[interface{}]interface{}{"userID": userID, "iat": now.Unix()}
	return session.Save(c.Request, c.Writer)
}

// sessionStillValid: false if the user's password changed after this session began.
func sessionStillValid(session *sessions.Session, u models.User) bool {
	if u.SessionsValidAfter == nil {
		return true
	}
	iat, _ := session.Values["iat"].(int64) // sessions from before this feature have none → 0
	return iat >= u.SessionsValidAfter.Unix()
}

func SaveSession(w http.ResponseWriter, r *http.Request, session *sessions.Session) error {
	return session.Save(r, w)
}
