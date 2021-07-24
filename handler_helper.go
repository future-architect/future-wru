package wru

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang/gddo/httputil"
)

func isHTML(r *http.Request) bool {
	return httputil.NegotiateContentType(r, []string{"text/html", "application/json"}, "text/html") == "text/html"
}

type loginInfoKeyType string

const (
	loginInfoKey loginInfoKeyType = "loginInfo"
)

type loginInfo struct {
	sid string
	ses *Session
}

func SetSessionInfo(r *http.Request, sid string, ses *Session) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), loginInfoKey, &loginInfo{
		sid: sid,
		ses: ses,
	}))
}

func GetSessionInfo(r *http.Request) (sid string, ses *Session) {
	if li, ok := r.Context().Value(loginInfoKey).(*loginInfo); ok {
		return li.sid, li.ses
	}
	return "", nil
}

func MustLogin(c *Config, s SessionStorage) func(http.Handler)http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sid, ses, ok := LookupSession(c, s, r)
			if !ok || (ses.Status != ActiveSession && ses.Status != BeforeLogin){
				StartSessionAndRedirect(c, s, w, r)
				return
			}
			next.ServeHTTP(w, SetSessionInfo(r, sid, ses))
		})
	}
}

func MustNotLogin(c *Config, s SessionStorage) func(http.Handler)http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sid, ses, ok := LookupSession(c, s, r)
			if ok && ses.Status == ActiveSession {
				http.Redirect(w, r, c.DefaultLandingPage, http.StatusFound)
				return
			}
			next.ServeHTTP(w, SetSessionInfo(r, sid, ses))
		})
	}

}

func SetSessionID(ctx context.Context, w http.ResponseWriter, sessionID string, c *Config, status SessionStatus) {
	now := currentTime(ctx)
	var expires time.Time
	if status == BeforeLogin {
		expires = now.Add(c.LoginTimeoutTerm)
	} else {
		expires = now.Add(c.SessionAbsoluteTimeoutTerm)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     c.ClientSessionKey,
		Value:    sessionID,
		Path:     "/",
		Domain:   "",
		Expires:  expires,
		Secure:   strings.HasPrefix(c.Host, "https://"),
		HttpOnly: c.ClientSessionFieldCookie == CookieField,
		SameSite: http.SameSiteLaxMode,
	})
}

func RemoveSessionID(w http.ResponseWriter, c *Config) {
	http.SetCookie(w, &http.Cookie{
		Name:     c.ClientSessionKey,
		Value:    "",
		Path:     "/",
		Domain:   "",
		Expires:  time.Date(1980,time.January, 1, 0, 0, 0, 0, time.UTC),
		Secure:   strings.HasPrefix(c.Host, "https://"),
		HttpOnly: c.ClientSessionFieldCookie == CookieField,
		SameSite: http.SameSiteLaxMode,
	})
}