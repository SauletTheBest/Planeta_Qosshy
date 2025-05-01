package middleware

import (
	"github.com/gorilla/sessions"
	"net/http"
	"os"
)

var store = sessions.NewCookieStore([]byte(os.Getenv("COOKIE_SECRET")))

func GetSession(r *http.Request) (*sessions.Session, error) {
	return store.Get(r, "session")
}

func SaveSession(w http.ResponseWriter, r *http.Request, session *sessions.Session) error {
	return session.Save(r, w)
}
