package wru

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

type contextTimeKey string

const timeKey contextTimeKey = "timeKey"

func currentTime(ctx context.Context) time.Time {
	v := ctx.Value(timeKey)
	if t, ok := v.(time.Time); ok {
		return t
	}
	return time.Now()
}

func setFixTime(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, timeKey, t)
}

func defaultConfig() *Config {
	return &Config{
		SessionIdleTimeoutTerm:     3 * time.Hour,
		SessionAbsoluteTimeoutTerm: 30 * 24 * time.Hour,
	}
}

func login(t *testing.T, userID string) (context.Context, *ServerlessSessionStorage, string, error) {
	t.Helper()

	s, err := NewMemorySessionStorage(context.Background(), defaultConfig(), xid.New().String())
	assert.NotNil(t, s)
	assert.NoError(t, err)

	now := time.Date(2021, time.July, 2, 10, 0, 0, 0, time.Local)
	ctx := setFixTime(context.Background(), now)

	oldSessionID, err := s.StartLogin(ctx, map[string]string{})

	r := httptest.NewRequest("GET", "/test", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.114 Safari/537.36")

	user := &User{
		DisplayName:           userID,
		Organization:          "secret",
		UserID:                userID,
		Email:                 userID + "@example.com",
		Scopes:                []string{"login"},
	}

	loginInfo := map[string]string{"login-idp": "debug"}
	sid, _, err := s.StartSession(ctx, oldSessionID, user, r, loginInfo)
	assert.NoError(t, err)
	assert.NotEqual(t, "", sid)
	return ctx, s, sid, err
}


func dummyUser(userID string) *User {
	user := &User{
		DisplayName:  userID,
		Organization: "secret",
		UserID:       userID,
		Email:        userID + "@example.com",
		Scopes:       []string{"login"},
	}
	return user
}

func dummyRequest() *http.Request {
	r := httptest.NewRequest("GET", "/test", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.114 Safari/537.36")
	return r
}