// warning: use OAuth for authentication is not secure

package wru

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gookit/color"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/garyburd/go-oauth/oauth"
)

var twitterClient *oauth.Client

func initTwitterClient(c *Config, out io.Writer) {
	if c.Twitter.Available() {
		twitterClient = &oauth.Client{
			TemporaryCredentialRequestURI: "https://api.twitter.com/oauth/request_token",
			ResourceOwnerAuthorizationURI: "https://api.twitter.com/oauth/authorize",
			TokenRequestURI:               "https://api.twitter.com/oauth/access_token",
			Credentials: oauth.Credentials{
				Token:  c.Twitter.ConsumerKey,
				Secret: c.Twitter.ConsumerSecret,
			},
		}
		c.availableIDPs["twitter"] = true
		if out != nil {
			color.Fprint(out, "<blue>Twitter Login:</> <green>OK</>\n")
		}
	} else if out != nil {
		color.Fprint(out, "<blue>Twitter Login:</> <red>NO</>\n")
	}
}

type twitterAccount struct {
	ID              string `json:"id_str"`
	ScreenName      string `json:"screen_name"`
	ProfileImageURL string `json:"profile_image_url"`
	Email           string `json:"email"`
}

func twitterLoginStart(c *Config) (redirectUrl string, loginInfo map[string]string, err error) {
	callback := strings.TrimSuffix(c.Host, "/") + "/.wru/callback"
	tc, err := twitterClient.RequestTemporaryCredentials(nil, callback, nil)
	if err != nil {
		err = fmt.Errorf("Twitter login error at getting temporary credential: %w", err)
		return
	}
	redirectUrl = twitterClient.AuthorizationURL(tc, nil)
	loginInfo = map[string]string{
		"idp":          "twitter",
		"token-key":    tc.Token,
		"token-secret": tc.Secret,
	}
	return redirectUrl, loginInfo, nil
}

func twitterCallback(c *Config, r *http.Request, loginInfo map[string]string) (twitterID string, newLoginInfo map[string]string, err error) {
	if err := r.ParseForm(); err != nil {
		return "", nil, fmt.Errorf("parse form error: %w", err)
	}

	tokenKey, ok1 := loginInfo["token-key"]
	secretKey, ok2 := loginInfo["token-secret"]
	if !ok1 || !ok2 {
		return "", nil, errors.New("internal server error: loginInfo is broken")
	}
	tc := &oauth.Credentials{
		Token:  tokenKey,
		Secret: secretKey,
	}

	if tc.Token != r.FormValue("oauth_token") {
		return "", nil, errors.New("internal server error: oauth_token missing")
	}

	cred, _, err := twitterClient.RequestToken(nil, tc, r.FormValue("oauth_verifier"))
	if err != nil {
		return "", nil, fmt.Errorf("getting access token error: %w", err)
	}

	resp, err := twitterClient.Get(nil, cred, "https://api.twitter.com/1.1/account/verify_credentials.json", url.Values{})

	if err != nil {
		return "", nil, fmt.Errorf("twitter login error error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return "", nil, fmt.Errorf("twitter is unavailable: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", nil, fmt.Errorf("internal server error: invalid request for twitter: %w", err)
	}

	var user twitterAccount
	err = json.NewDecoder(resp.Body).Decode(&user)
	if err != nil {
		return "", nil, fmt.Errorf("internal server error: json decode error: %w", err)
	}
	twitterID = user.ScreenName
	newLoginInfo = map[string]string{
		"login-idp": "twitter",
		// "twitter-secret": cred.Secret,
		// "twitter-token": cred.Token,
	}
	return
}
