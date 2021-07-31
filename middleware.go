package wru

import (
	"context"
	"fmt"
	"github.com/gookit/color"
	"io"
	"net/http"
	"os"
)

func NewAuthorizationMiddleware(ctx context.Context, c *Config, out io.Writer) (http.Handler, func(http.Handler) http.Handler) {
	err := c.Init(ctx, out)
	if err != nil {
		fmt.Fprintln(os.Stderr, color.Error.Sprintf("Config validation error: %s", err.Error()))
		os.Exit(1)
	}
	sessionStorage, err := NewSessionStorage(ctx, c, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, color.Error.Sprintf("Connect session error: %s", err.Error()))
		os.Exit(1)
	}
	identityRegister, warnings, err := NewIdentityRegister(ctx, c, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, color.Error.Sprintf("Read user table error: %s", err.Error()))
		os.Exit(1)
	}
	for _, u := range c.Users {
		identityRegister.appendUser(u)
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, color.Warn.Sprintf("User parse warning: %s", w))
	}
	handler := newHandler(c, sessionStorage, identityRegister)
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sid, ses, ok := lookupSessionFromRequest(c, sessionStorage, r)
			if !ok || (ses.Status != ActiveSession) {
				if r.RequestURI == "/favicon.ico" {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				startSessionAndRedirect(c, sessionStorage, w, r)
				return
			}
			next.ServeHTTP(w, setSessionInfo(r, sid, ses))
			sessionStorage.UpdateSessionData(r.Context(), sid, ses.directrives)

		})
	}
	return handler, middleware
}
