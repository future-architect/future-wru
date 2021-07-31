package wru

import (
	"log"
	"net/http"
)

func startSessionAndRedirect(c *Config, s SessionStorage, w http.ResponseWriter, r *http.Request) {
	sessionID, err := s.StartLogin(r.Context(), map[string]string{
		"landingURL": r.RequestURI,
	})
	log.Printf("🥚 start login: %s %s\n", sessionID, r.RequestURI)
	if err != nil {
		http.Error(w, "internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	setSessionID(r.Context(), w, sessionID, c, BeforeLogin)
	http.Redirect(w, r, "/.wru/login", http.StatusFound)
	return
}

func lookupSessionFromRequest(c *Config, s SessionStorage, r *http.Request) (string, *Session, bool) {
	var sessionID string
	for _, ck := range r.Cookies() {
		if ck.Name == c.ClientSessionKey {
			sessionID = ck.Value
			break
		}
	}
	if sessionID != "" {
		ses, err := s.FindBySessionToken(r.Context(), sessionID)
		if err == nil {
			return sessionID, ses, true
		}
	}
	return "", nil, false
}
