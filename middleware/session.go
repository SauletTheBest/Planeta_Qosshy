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

func GetStore() *sessions.CookieStore {
	once.Do(func() {
		_ = godotenv.Load()
		secret := os.Getenv("COOKIE_SECRET")
		if secret == "" {
			secret = "default_secret_key_planeta_qosshy_2026"
		}
		store = sessions.NewCookieStore([]byte(secret))
	})
	return store
}

func GetSession(r *http.Request) (*sessions.Session, error) {
	return GetStore().Get(r, "session")
}

func SaveSession(w http.ResponseWriter, r *http.Request, session *sessions.Session) error {
	return session.Save(r, w)
}
