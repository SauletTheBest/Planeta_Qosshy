package middleware

import (
	"net/http"
	"os"
	"sync"

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

func SaveSession(w http.ResponseWriter, r *http.Request, session *sessions.Session) error {
	return session.Save(r, w)
}
