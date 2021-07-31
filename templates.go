package wru

import (
	"embed"
	"html/template"
	"io"
	"path/filepath"
)

//go:embed templates/*.html
var defaultTemplates embed.FS

const (
	LoginPageTemplate        = "login.html"
	DebugLoginPageTemplate   = "debug_login.html"
	UserStatusPageTemplate   = "user_status.html"
	UserSessionsPageTemplate = "user_sessions.html"
)

var pages *template.Template

type loginPageContext struct {
	GitHub  bool
	Twitter bool
	OIDC    bool
}

type debugLoginPageContext struct {
	Users []*User
}

func initTemplate(c *Config, out io.Writer) error {
	var err error
	if c.HTMLTemplateFolder != "" {
		pages, err = template.ParseGlob(filepath.Join(c.HTMLTemplateFolder, "*"))
	} else {
		pages, err = template.ParseFS(defaultTemplates, "templates/*.html")
	}
	if err != nil {
		return err
	}
	return nil
}
