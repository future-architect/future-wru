// warning: use OAuth for authentication is not secure

package wru

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
	"github.com/gookit/color"
	"github.com/shibukawa/uuid62"
	"golang.org/x/oauth2"
	githubEndpoint "golang.org/x/oauth2/github"
)

var githubClient *oauth2.Config

func initGitHubConfig(c *Config, out io.Writer) {
	if c.GitHub.Available() {
		callback := strings.TrimSuffix(c.Host, "/") + "/.wru/callback"
		githubClient = &oauth2.Config{
			ClientID:     c.GitHub.ClientID,
			ClientSecret: c.GitHub.ClientSecret,
			Endpoint:     githubEndpoint.Endpoint,
			Scopes:       []string{"user:email"},
			RedirectURL:  callback,
		}
		c.AvailableIDPs["github"] = true
		if out != nil {
			color.Fprint(out, "<blue>GitHub Login:</> <green>OK</>\n")
		}
	} else if out != nil {
		color.Fprint(out, "<blue>GitHub Login:</> <red>NO</>\n")
	}
}

func gitHubLoginStart(c *Config) (redirectUrl string, loginInfo map[string]string, err error) {
	state, err := uuid62.V4()
	if err != nil {
		return "", nil, err
	}
	redirectUrl = githubClient.AuthCodeURL(state)
	loginInfo = map[string]string{
		"idp":   "github",
		"state": state,
	}
	return
}

func githubCallback(c *Config, r *http.Request, loginInfo map[string]string) (gitHubID string, newLoginInfo map[string]string, err error) {
	if err := r.ParseForm(); err != nil {
		return "", nil, fmt.Errorf("parse form error: %w", err)
	}

	if loginInfo["state"] != r.Form.Get("state") {
		err = errors.New("state is different")
		return
	}

	token, err := githubClient.Exchange(context.Background(), r.Form.Get("code"))
	if err != nil {
		err = fmt.Errorf("can't get access token: %w", err)
		return
	}

	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token.AccessToken},
	)

	client := github.NewClient(oauth2.NewClient(context.Background(), tokenSource))

	user, _, err := client.Users.Get(context.Background(), "")
	if err != nil {
		err = fmt.Errorf("can't get access token: %w", err)
		return
	}

	gitHubID = user.GetLogin()
	newLoginInfo = map[string]string{
		"login-idp": "github",
		// "github-refresh": token.RefreshToken,
		// "github-token": token.AccessToken,
	}
	return
}
