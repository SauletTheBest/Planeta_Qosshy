package middleware

import (
	"net/http"
	"os"
	"strings"

	"planeta_qosshy/util"

	"github.com/gin-gonic/gin"
)

// CrossOriginGuard rejects form submissions that come from another website (CSRF).
//
// The attack: you are logged in to the shop and open an evil page. It contains a hidden
// <form action="https://our-shop/admin/users/7/role" method="POST"> that submits itself.
// Your browser adds your session cookie, and the shop thinks YOU sent the form.
//
// Modern browsers say where a request comes from: the Sec-Fetch-Site header
// ("same-origin" / "cross-site"), or at least Origin. http.CrossOriginProtection (new in
// Go 1.25) checks them on every POST/PUT/PATCH/DELETE: from our own pages — allowed,
// from another site — refused. A request without these headers does not come from a web
// page at all (the bank's webhook, curl), so it is not a CSRF attack and passes.
//
// It works together with SameSite=Lax on the session cookie: two independent locks.
// The classic alternative, a hidden random token in every form, needs a change in every
// template and handler; this check needs none.
func CrossOriginGuard() gin.HandlerFunc {
	cop := http.NewCrossOriginProtection()

	// Our own public address is always ours, even when a proxy in front changes the Host header.
	origins := util.SplitList(os.Getenv("TRUSTED_ORIGINS"))
	if public := strings.TrimRight(os.Getenv("PUBLIC_URL"), "/"); public != "" {
		origins = append(origins, public)
	}
	for _, origin := range origins {
		if err := cop.AddTrustedOrigin(origin); err != nil {
			log.Fatalf("TRUSTED_ORIGINS / PUBLIC_URL: %v", err)
		}
	}

	return func(c *gin.Context) {
		if err := cop.Check(c.Request); err != nil {
			log.WithField("origin", c.GetHeader("Origin")).
				WithField("sec_fetch_site", c.GetHeader("Sec-Fetch-Site")).
				Warn("cross-origin request blocked")
			c.HTML(http.StatusForbidden, "error.html", gin.H{
				"error": "Запрос отклонён: форма была отправлена с другого сайта.",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
