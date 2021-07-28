package wru

import (
	"context"
	"errors"
	"fmt"
	"github.com/coreos/go-oidc"
	"github.com/gookit/color"
	"github.com/shibukawa/uuid62"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"strings"
)

var provider *oidc.Provider
var oauth2Config *oauth2.Config

func initOpenIDConnectConfig(ctx context.Context, c *Config, out io.Writer) error {
	if c.OIDC.Available() {
		var err error
		// ここにissuer情報を設定
		provider, err = oidc.NewProvider(ctx, c.OIDC.ProviderURL)
		if err != nil {
			return err
		}
		callback := strings.TrimSuffix(c.Host, "/") + "/.wru/callback"
		oauth2Config = &oauth2.Config{
			// ここにクライアントIDとクライアントシークレットを設定
			ClientID:     c.OIDC.ClientID,
			ClientSecret: c.OIDC.ClientSecret,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID},
			RedirectURL:  callback,
		}
		if out != nil {
			color.Fprint(out, "<blue>OpenID Connect Login:</> <green>OK</>\n")
		}
	} else if out != nil {
		color.Fprint(out, "<blue>OpenID Connect Login:</> <red>NO</>\n")
	}
	return nil
}

func oidcLoginStart(c *Config) (redirectUrl string, loginInfo map[string]string, err error) {
	state, err := uuid62.V4()
	if err != nil {
		return "", nil, err
	}
	redirectUrl = oauth2Config.AuthCodeURL(state)
	loginInfo = map[string]string{
		"idp":   "oidc",
		"state": state,
	}
	return
}

func oidcCallback(c *Config, r *http.Request, loginInfo map[string]string) (oidcID string, newLoginInfo map[string]string, err error) {
	if err := r.ParseForm(); err != nil {
		return "", nil, fmt.Errorf("parse form error: %w", err)
	}

	if loginInfo["state"] != r.Form.Get("state") {
		err = errors.New("state is different")
		return
	}

	nonce, err := uuid62.V4()
	if err != nil {
		return "", nil, err
	}

	accessToken, err := oauth2Config.Exchange(context.Background(), r.Form.Get("code"), oidc.Nonce(nonce))
	if err != nil {
		err = fmt.Errorf("can't get access token: %w", err)
		return
	}

	rawIDToken, ok := accessToken.Extra("id_token").(string)
	if !ok {
		err = fmt.Errorf("id token missing")
		return
	}

	oidcConfig := &oidc.Config{
		ClientID: c.OIDC.ClientID,
	}
	verifier := provider.Verifier(oidcConfig)
	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		err = fmt.Errorf("id token verify error: %v", err)
		return
	}
	// IDトークンのクレームをとりあえずダンプ
	// アプリで必要なものはセッションストレージに入れておくと良いでしょう
	idTokenClaims := map[string]interface{}{}
	if err := idToken.Claims(&idTokenClaims); err != nil {
		return "", nil, fmt.Errorf("getting claims from id token error: %v", err)
	}
	aud, ok := idTokenClaims["aud"].(string)
	if !ok || aud != c.OIDC.ClientID {
		return "", nil, fmt.Errorf("this code is not for this service: %s", aud)
	}
	if nonce2, ok := idTokenClaims["nonce"].(string); ok {
		if nonce2 != nonce {
			return "", nil, fmt.Errorf("exchange id token error")
		}
	}
	oidcID, ok = idTokenClaims["email"].(string)
	if !ok {
		oidcID = idTokenClaims["sub"].(string)
	}
	newLoginInfo = map[string]string{
		"login-idp": "oidc",
		// "github-refresh": token.RefreshToken,
		// "github-token": token.AccessToken,
	}
	return
}
