package wru

import (
	"io"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type wruHandler struct {
	c  *Config
	s  SessionStorage
	ir *IdentityRegister
}

func (wh wruHandler) Login(w http.ResponseWriter, r *http.Request) {
	if wh.c.DevMode {
		pages.ExecuteTemplate(w, "debug_login.html", &debugLoginPageContext{
			Users: wh.ir.AllUsers(),
		})
	} else {
		pages.ExecuteTemplate(w, "login.html", &loginPageContext{
			Twitter: wh.c.Twitter.Available(),
			GitHub:  wh.c.GitHub.Available(),
			OIDC:    wh.c.OIDC.Available(),
		})
	}
}

func (wh wruHandler) DebugLogin(w http.ResponseWriter, r *http.Request) {
	id, _ := GetSession(r)
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "http request error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	userID := r.Form.Get("userid")
	user, err := wh.ir.FindUserByID(userID)
	if err != nil {
		http.Error(w, "user not found: "+userID, http.StatusNotFound)
	}
	loginInfo := map[string]string{
		"login-idp": "debug",
	}
	newID, oldInfo, err := wh.s.StartSession(r.Context(), id, user, r, loginInfo)
	if err != nil {
		http.Error(w, "login error: "+err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("🐣 login as %s\n", userID)
	setSessionID(r.Context(), w, newID, wh.c, ActiveSession)
	if u, ok := oldInfo["landingURL"]; ok {
		http.Redirect(w, r, u, http.StatusFound)
	} else {
		http.Redirect(w, r, wh.c.DefaultLandingPage, http.StatusFound)
	}
}

func (wh wruHandler) FederatedLogin(w http.ResponseWriter, r *http.Request) {
	idp := chi.URLParam(r, "provider")
	oldSessionID, _ := GetSession(r)
	if oldSessionID == "" {
		var err error
		oldSessionID, err = wh.s.StartLogin(r.Context(), map[string]string{
			"landingURL": wh.c.DefaultLandingPage,
		})
		if err != nil {
			http.Error(w, "session storage access error: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	var redirectUrl string
	var loginInfo map[string]string
	var err error
	switch idp {
	case "twitter":
		if !wh.c.Twitter.Available() {
			http.Error(w, "Twitter login is not configured", http.StatusBadRequest)
			return
		}
		redirectUrl, loginInfo, err = twitterLoginStart(wh.c)
	case "github":
		if !wh.c.GitHub.Available() {
			http.Error(w, "GitHub login is not configured", http.StatusBadRequest)
			return
		}
		redirectUrl, loginInfo, err = gitHubLoginStart(wh.c)
	case "oidc":
		if !wh.c.OIDC.Available() {
			http.Error(w, "OpenID Connect login is not configured", http.StatusBadRequest)
			return
		}
		redirectUrl, loginInfo, err = oidcLoginStart(wh.c)
	default:
		http.Error(w, "undefined provider: "+idp, http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "can't start login sequence: "+err.Error(), http.StatusInternalServerError)
		return
	}
	newSessionID, err := wh.s.AddLoginInfo(r.Context(), oldSessionID, loginInfo)
	if err != nil {
		http.Error(w, "session storage access error: "+err.Error(), http.StatusBadRequest)
		return
	}
	setSessionID(r.Context(), w, newSessionID, wh.c, BeforeLogin)
	http.Redirect(w, r, redirectUrl, http.StatusFound)
}

func (wh wruHandler) Callback(w http.ResponseWriter, r *http.Request) {
	id, ses, _ := lookupSessionFromRequest(wh.c, wh.s, r)
	idpName := ses.Data["idp"]
	var idpUser string
	var err error
	var idp IDPlatform
	var newLoginInfo map[string]string
	switch idpName {
	case "twitter":
		if !wh.c.Twitter.Available() {
			http.Error(w, "Twitter login is not configured", http.StatusBadRequest)
			return
		}
		idpUser, newLoginInfo, err = twitterCallback(wh.c, r, ses.Data)
		idp = Twitter
	case "github":
		if !wh.c.GitHub.Available() {
			http.Error(w, "GitHub login is not configured", http.StatusBadRequest)
			return
		}
		idp = GitHub
		idpUser, newLoginInfo, err = githubCallback(wh.c, r, ses.Data)
	case "oidc":
		if !wh.c.GitHub.Available() {
			http.Error(w, "OpenID Connect login is not configured", http.StatusBadRequest)
			return
		}
		idp = OIDC
		idpUser, newLoginInfo, err = oidcCallback(wh.c, r, ses.Data)
	default:
		http.Error(w, "undefined provider: "+idpName, http.StatusBadRequest)
		return
	}

	user, err := wh.ir.FindUserOf(idp, idpUser)
	if err != nil {
		http.Error(w, "user not found: "+idpUser+" of "+idpName, http.StatusNotFound)
		return
	}

	newID, oldInfo, err := wh.s.StartSession(r.Context(), id, user, r, newLoginInfo)
	setSessionID(r.Context(), w, newID, wh.c, ActiveSession)
	log.Printf("🐣 login as %s of %s\n", idpUser, idpName)
	setSessionID(r.Context(), w, newID, wh.c, ActiveSession)
	if u, ok := oldInfo["landingURL"]; ok {
		http.Redirect(w, r, u, http.StatusFound)
	} else {
		http.Redirect(w, r, wh.c.DefaultLandingPage, http.StatusFound)
	}
}

func (wh wruHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusInternalServerError)
}

func (wh wruHandler) ConfirmAction(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusInternalServerError)
}

func (wh wruHandler) Logout(w http.ResponseWriter, r *http.Request) {
	id, _ := GetSession(r)
	err := wh.s.Logout(r.Context(), id)
	if err != nil {
		if isHTML(r) {
			http.Redirect(w, r, "/.wru/login?logout_error", http.StatusFound)
		} else {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"status": "error"}`)
		}
		return
	}
	removeSessionID(w, wh.c)
	if isHTML(r) {
		http.Redirect(w, r, "/.wru/login", http.StatusFound)
	} else {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		io.WriteString(w, `{"status": "ok"}`)
	}
}

func (wh wruHandler) User(w http.ResponseWriter, r *http.Request) {
	_, ses := GetSession(r)
	u, err := wh.ir.FindUserByID(ses.UserID)

	if err != nil {
		http.Error(w, "user not found: "+ses.UserID, http.StatusNotFound)
		return
	}

	if isHTML(r) {
		pages.ExecuteTemplate(w, "user_status.html", u)
	} else {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		u.WriteAsJson(w)
	}
}

func (wh wruHandler) Sessions(w http.ResponseWriter, r *http.Request) {
	sid, ses := GetSession(r)
	sessions, err := wh.s.GetUserSessions(r.Context(), ses.UserID)
	if err != nil {
		http.Error(w, "user not found: "+ses.UserID, http.StatusNotFound)
		return
	}
	for i, s := range sessions {
		if s.ID == sid {
			sessions[i].CurrentSession = true
			break
		}
	}

	if isHTML(r) {
		pages.ExecuteTemplate(w, "user_sessions.html", sessions)
	} else {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		AllUserSessions(sessions).WriteAsJson(w)
	}
}

func (wh wruHandler) SessionLogout(w http.ResponseWriter, r *http.Request) {
	currentID, _ := GetSession(r)
	targetID := chi.URLParam(r, "sessionID")
	if currentID == targetID {
		http.Error(w, "target session ID should not be as same as current ID", http.StatusBadRequest)
		return
	}
	err := wh.s.Logout(r.Context(), targetID)
	if err != nil {
		if isHTML(r) {
			http.Redirect(w, r, "/.wru/user/sessions?logout_error", http.StatusFound)
		} else {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"status": "error"}`)
		}
		return
	}
	if isHTML(r) {
		http.Redirect(w, r, "/.wru/user/sessions", http.StatusFound)
	} else {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		io.WriteString(w, `{"status": "ok"}`)
	}
}

func authMiddleware(c *Config, s SessionStorage, u *IdentityRegister) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		r := newHandler(c, s, u)
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			sid, ses, ok := lookupSessionFromRequest(c, s, r)
			if !ok || (ses.Status != ActiveSession) {
				if r.RequestURI == "/favicon.ico" {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				startSessionAndRedirect(c, s, w, r)
				return
			}
			next.ServeHTTP(w, setSessionInfo(r, sid, ses))
		})
		return r
	}
}

func newHandler(c *Config, s SessionStorage, u *IdentityRegister) *chi.Mux {
	wh := &wruHandler{
		c:  c,
		s:  s,
		ir: u,
	}
	r := chi.NewRouter()
	r.Route("/.wru", func(r chi.Router) {
		r.With(MustNotLogin(c, s)).Get("/login", wh.Login)
		if c.DevMode {
			r.With(MustNotLogin(c, s)).Post("/login", wh.DebugLogin)
		} else {
			r.With(MustNotLogin(c, s)).Get("/login/{provider}", wh.FederatedLogin)
			r.With(MustNotLogin(c, s)).Get("/callback", wh.Callback)
		}
		r.With(MustLogin(c, s)).Get("/logout", wh.Logout)
		r.With(MustLogin(c, s)).Get("/user", wh.User)
		r.With(MustLogin(c, s)).Get("/user/sessions", wh.Sessions)
		r.With(MustLogin(c, s)).Post("/user/sessions/{sessionID}/logout", wh.SessionLogout)
	})
	return r
}

func NewIdentityAwareProxyHandler(c *Config, s SessionStorage, u *IdentityRegister) (http.Handler, error) {
	h, err := NewReverseProxy(c, s)
	if err != nil {
		return nil, err
	}
	return authMiddleware(c, s, u)(h), nil
}
