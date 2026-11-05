package middleware

import "github.com/gin-gonic/gin"

// Flash is a one-time message shown on the next page, e.g. after
// POST /admin/orders/5/status → redirect → GET /admin/orders/5: "Заказ №5 отправлен".
// The redirect (Post/Redirect/Get) keeps F5 from repeating the action, but it also
// loses everything the handler wanted to say. So the message rides in the session cookie
// and is deleted as soon as it has been read once.
type Flash struct {
	Kind string // success | error | info  (= CSS class alert-<kind>)
	Text string
}

var flashKinds = []string{"error", "success", "info"}

// SetFlash stores a message for the next page. Call it before c.Redirect.
func SetFlash(c *gin.Context, kind, text string) {
	session, err := GetSession(c.Request)
	if err != nil {
		return
	}
	// Only the latest message is kept: an old unread one would pop up later, out of place.
	for _, k := range flashKinds {
		session.Flashes("flash_" + k)
	}
	session.AddFlash(text, "flash_"+kind)
	if err := session.Save(c.Request, c.Writer); err != nil {
		log.WithError(err).Warn("flash: session save")
	}
}

// PopFlash returns the waiting message (or nil) and removes it from the session.
// Call it before rendering: the cookie has to be rewritten before the page is sent.
func PopFlash(c *gin.Context) *Flash {
	session, err := GetSession(c.Request)
	if err != nil {
		return nil
	}
	for _, kind := range flashKinds {
		if msgs := session.Flashes("flash_" + kind); len(msgs) > 0 {
			_ = session.Save(c.Request, c.Writer) // Flashes() removed it; saving makes that stick
			if text, ok := msgs[0].(string); ok {
				return &Flash{Kind: kind, Text: text}
			}
		}
	}
	return nil
}
