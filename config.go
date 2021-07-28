package wru

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gookit/color"
	"github.com/kelseyhightower/envconfig"
	"github.com/oschwald/geoip2-golang"
)

type ClientSessionFieldType int

const (
	CookieField ClientSessionFieldType = iota + 1
	CookieWithJSField
	InvalidField
)

type configFromEnv struct {
	Port      uint16 `envconfig:"PORT" default:"3000"`
	Host      string `envconfig:"HOST" required:"true"`
	AdminPort uint16 `envconfig:"ADMIN_PORT" default:"3001"` // todo: implment admin screen

	DevMode               bool   `envconfig:"WRU_DEV_MODE" default:"true"`
	TlsCert               string `envconfig:"WRU_TLS_CERT"`
	TlsKey                string `envconfig:"WRU_TLS_KEY"`
	ForwardTo             string `envconfig:"WRU_FORWARD_TO" required:"true"`
	DefaultLandingPage    string `envconfig:"WRU_DEFAULT_LANDING_PAGE" default:"/"`
	SessionStorage        string `envconfig:"WRU_SESSION_STORAGE"`
	ClientSessionIDCookie string `envconfig:"WRU_CLIENT_SESSION_ID_COOKIE" default:"WRU_SESSION@cookie"`
	ServerSessionField    string `envconfig:"WRU_SERVER_SESSION_FIELD" default:"Wru-Session"`

	UserTable           string        `envconfig:"WRU_USER_TABLE"`
	UserTableReloadTerm time.Duration `envconfig:"WRU_USER_TABLE_RELOAD_TERM"`

	LoginTimeoutTerm           time.Duration `envconfig:"WRU_LOGIN_TIMEOUT_TERM" default:"10m"`
	SessionIdleTimeoutTerm     time.Duration `envconfig:"WRU_SESSION_IDLE_TIMEOUT_TERM" default:"1h"`
	SessionAbsoluteTimeoutTerm time.Duration `envconfig:"WRU_SESSION_ABSOLUTE_TIMEOUT_TERM" default:"720h"`

	HTMLTemplateFolder string `envconfig:"WRU_HTML_TEMPLATE_FOLDER"`

	TwitterConsumerKey    string `envconfig:"WRU_TWITTER_CONSUMER_KEY"`
	TwitterConsumerSecret string `envconfig:"WRU_TWITTER_CONSUMER_SECRET"`

	GitHubClientID     string `envconfig:"WRU_GITHUB_CLIENT_ID"`
	GitHubClientSecret string `envconfig:"WRU_GITHUB_CLIENT_SECRET"`

	OIDCProviderURL  string `envconfig:"WRU_OIDC_PROVIDER_URL"`
	OIDCClientID     string `envconfig:"WRU_OIDC_CLIENT_ID"`
	OIDCClientSecret string `envconfig:"WRU_OIDC_CLIENT_SECRET"`

	GeoIPDatabase string `envconfig:"WRU_GEIIP_DATABASE"`
}

type Config struct {
	Port uint16
	Host string

	DevMode bool

	AdminPort                uint16
	TlsCert                  string
	TlsKey                   string
	ForwardTo                []Route
	DefaultLandingPage       string
	UserTable                string
	UserTableReloadTerm      time.Duration
	SessionStorage           string
	ServerSessionField       string
	ClientSessionFieldCookie ClientSessionFieldType
	ClientSessionKey         string

	LoginTimeoutTerm           time.Duration
	SessionIdleTimeoutTerm     time.Duration
	SessionAbsoluteTimeoutTerm time.Duration

	HTMLTemplateFolder string

	Twitter TwitterConfig
	GitHub  GitHubConfig
	OIDC    OIDCConfig

	AvailableIDPs map[string]bool

	RedisSession RedisConfig

	GeoIPDB *geoip2.Reader
}

func NewConfigFromEnv(ctx context.Context, out io.Writer) (*Config, error) {
	var e configFromEnv
	err := envconfig.Process("", &e)
	if err != nil {
		return nil, err
	}

	routes, err := parseForwardList(e.ForwardTo)
	if err != nil {
		return nil, err
	}
	fieldKey, fieldType, err := parseClientSessionField(e.ClientSessionIDCookie)
	if err != nil {
		return nil, err
	}

	var db *geoip2.Reader
	if e.GeoIPDatabase != "" {
		db, err = geoip2.Open(e.GeoIPDatabase)
		if err != nil {
			return nil, err
		}
	}

	c := Config{
		Port:                       e.Port,
		Host:                       e.Host,
		AdminPort:                  e.AdminPort,
		TlsCert:                    e.TlsCert,
		TlsKey:                     e.TlsKey,
		UserTable:                  e.UserTable,
		UserTableReloadTerm:        e.UserTableReloadTerm,
		ForwardTo:                  routes,
		DefaultLandingPage:         e.DefaultLandingPage,
		SessionStorage:             e.SessionStorage,
		ServerSessionField:         e.ServerSessionField,
		ClientSessionKey:           fieldKey,
		ClientSessionFieldCookie:   fieldType,
		LoginTimeoutTerm:           e.LoginTimeoutTerm,
		SessionIdleTimeoutTerm:     e.SessionIdleTimeoutTerm,
		SessionAbsoluteTimeoutTerm: e.SessionAbsoluteTimeoutTerm,
		HTMLTemplateFolder:         e.HTMLTemplateFolder,
		Twitter: TwitterConfig{
			ConsumerKey:    e.TwitterConsumerKey,
			ConsumerSecret: e.TwitterConsumerSecret,
		},
		GitHub: GitHubConfig{
			ClientID:     e.GitHubClientID,
			ClientSecret: e.GitHubClientSecret,
		},
		OIDC: OIDCConfig{
			ProviderURL:  e.OIDCProviderURL,
			ClientID:     e.OIDCClientID,
			ClientSecret: e.OIDCClientSecret,
		},
		DevMode:       e.DevMode,
		AvailableIDPs: make(map[string]bool),
		GeoIPDB: db,
	}

	if out != nil {
		color.Fprintf(out, "<blue>Host:</> %s\n", e.Host)
		color.Fprintf(out, "<blue>Port:</> %d\n", e.Port)
		if c.TlsCert != "" && c.TlsKey != "" {
			color.Fprintf(out, "<blue>TLS:</> <green>enabled</>\n")
		} else {
			color.Fprintf(out, "<blue>TLS:</> <red>disabled</>\n")
		}
		color.Fprintf(out, "<blue>DevMode:</> <red>%v</>\n", c.DevMode)
		color.Fprintf(out, "<blue>Forward To:</>\n")
		for _, r := range c.ForwardTo {
			color.Fprintf(out, "  <green>%s</> => %s (%s)\n", r.Path, r.Host.String(), strings.Join(r.Scopes, ", "))
		}
		if c.GeoIPDB != nil {
			color.Fprintf(out, "<blue>GeoIP:</> <green>enabled(%s)</>\n", e.GeoIPDatabase)
		} else {
			color.Fprintf(out, "<blue>GeoIP:</> <red>disabled</>\n")
		}
	}

	if !c.DevMode {
		initTwitterClient(&c, out)
		initGitHubConfig(&c, out)
		initOpenIDConnectConfig(ctx, &c, out)
		if len(c.AvailableIDPs) == 0 {
			return nil, errors.New("No ID Provider is available")
		}
	}
	return &c, nil
}

var rre = regexp.MustCompile(`\s*(/.*)\s*=>\s*(https?://[^\s (]+)(\s*\((.*)\))?\s*`)

func parseForwardList(src string) ([]Route, error) {
	var result []Route
	for i, route := range strings.Split(src, ";") {
		if strings.TrimSpace(route) == "" {
			continue
		}
		match := rre.FindStringSubmatch(route)
		if len(match) == 0 {
			return nil, fmt.Errorf("wrong route definition: (%d)=%s", i, route)
		}
		var scopes []string
		for _, s := range strings.Split(match[4], ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				scopes = append(scopes, s)
			}
		}
		u, err := url.Parse(match[2])
		if err != nil {
			return nil, err
		}
		result = append(result, Route{
			Path:   strings.TrimSpace(match[1]),
			Host:   u,
			Scopes: scopes,
		})
	}
	return result, nil
}

func parseClientSessionField(src string) (string, ClientSessionFieldType, error) {
	fragments := strings.SplitN(src, "@", 2)
	if len(fragments) == 1 {
		return src, CookieField, nil
	}
	if fragments[1] == "cookie" {
		return fragments[0], CookieField, nil
	} else if fragments[1] == "cookie-with-js" {
		return fragments[0], CookieWithJSField, nil
	}
	return "", InvalidField, errors.New("invalid client session field")
}

type Route struct {
	Path   string
	Host   *url.URL
	Scopes []string
}

type TwitterConfig struct {
	ConsumerKey    string
	ConsumerSecret string
}

func (c TwitterConfig) Available() bool {
	return c.ConsumerKey != "" && c.ConsumerSecret != ""
}

type GitHubConfig struct {
	ClientID     string
	ClientSecret string
}

func (c GitHubConfig) Available() bool {
	return c.ClientID != "" && c.ClientSecret != ""
}

type OIDCConfig struct {
	ProviderURL  string
	ClientID     string
	ClientSecret string
}

func (c OIDCConfig) Available() bool {
	return c.ProviderURL != "" && c.ClientID != "" && c.ClientSecret != ""
}

type RedisConfig struct {
	Host string
}
