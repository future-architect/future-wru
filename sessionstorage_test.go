package wru

import (
	"context"
	_ "gocloud.dev/docstore/memdocstore"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func TestFederatedLoginSuccess(t *testing.T) {
	s, err := NewMemorySessionStorage(context.Background(), defaultConfig(), xid.New().String())
	assert.NotNil(t, s)
	assert.NoError(t, err)

	now := time.Date(2021, time.July, 2, 10, 0, 0, 0, time.Local)
	ctx := setFixTime(context.Background(), now)

	firstSessionID, err := s.StartLogin(ctx, map[string]string{
		"redirectURL": "/",
	})
	assert.NotEqual(t, "", firstSessionID)

	secondSessionID, err := s.AddLoginInfo(ctx, firstSessionID, map[string]string{
		"provider": "twitter",
	})
	assert.NoError(t, err)
	assert.NotEqual(t, firstSessionID, secondSessionID)

	ses, err := s.FindBySessionToken(ctx, secondSessionID)
	assert.NoError(t, err)
	assert.NotNil(t, ses)

	r := dummyRequest()
	user := dummyUser("user1")

	newSessionID, loginInfo, err := s.StartSession(ctx, secondSessionID, user, r)
	assert.NoError(t, err)
	assert.NotEqual(t, "", newSessionID)
	assert.NotEqual(t, firstSessionID, newSessionID)
	assert.NotEqual(t, secondSessionID, newSessionID)
	assert.Equal(t, "/", loginInfo["redirectURL"])
	assert.Equal(t, "twitter", loginInfo["provider"])
}

func TestDebugLoginSuccess(t *testing.T) {
	s, err := NewMemorySessionStorage(context.Background(), defaultConfig(), xid.New().String())
	assert.NotNil(t, s)
	assert.NoError(t, err)

	now := time.Date(2021, time.July, 2, 10, 0, 0, 0, time.Local)
	ctx := setFixTime(context.Background(), now)

	oldSessionID, err := s.StartLogin(ctx, map[string]string{"redirectURL": "/"})
	assert.NotEqual(t, "", oldSessionID)

	ses, err := s.FindBySessionToken(ctx, oldSessionID)
	assert.NoError(t, err)
	assert.NotNil(t, ses)

	r := dummyRequest()
	user := dummyUser("user1")

	newSessionID, loginInfo, err := s.StartSession(ctx, oldSessionID, user, r)
	assert.NoError(t, err)
	assert.NotEqual(t, "", newSessionID)
	assert.NotEqual(t, oldSessionID, newSessionID)
	assert.Equal(t, "/", loginInfo["redirectURL"])
}

func TestLoginFail_NoStartLogin(t *testing.T) {
	s, err := NewMemorySessionStorage(context.Background(), defaultConfig(), xid.New().String())
	assert.NotNil(t, s)
	assert.NoError(t, err)

	now := time.Date(2021, time.July, 2, 10, 0, 0, 0, time.Local)
	ctx := setFixTime(context.Background(), now)

	r := dummyRequest()
	user := dummyUser("user1")

	_, _, err = s.StartSession(ctx, "invalid-session", user, r)

	assert.Error(t, err)
}

func TestLoginFail_StartSessionTwice(t *testing.T) {
	s, err := NewMemorySessionStorage(context.Background(), defaultConfig(), xid.New().String())
	assert.NotNil(t, s)
	assert.NoError(t, err)

	now := time.Date(2021, time.July, 2, 10, 0, 0, 0, time.Local)
	ctx := setFixTime(context.Background(), now)

	oldSessionID, err := s.StartLogin(ctx, map[string]string{"redirectURL": "/"})
	assert.NotEqual(t, "", oldSessionID)

	r := dummyRequest()
	user := dummyUser("user1")

	_, _, err = s.StartSession(ctx, oldSessionID, user, r)
	assert.NoError(t, err)
	_, _, err = s.StartSession(ctx, oldSessionID, user, r)
	assert.Error(t, err)
}

func TestSessionStorage_SingleSession(t *testing.T) {
	ctx, s, sid, err := login(t, "user1")
	assert.NoError(t, err)
	defer s.Close()

	now := currentTime(ctx)

	ses, err := s.FindBySessionToken(ctx, sid)
	assert.NotNil(t, ses)
	assert.NoError(t, err)
	assert.Equal(t, "user1", ses.UserID)
	assert.Equal(t, now, time.Time(ses.LoginAt))
	assert.Equal(t, ActiveSession, ses.Status)
}

func TestSessionStorage_SessionNotFound(t *testing.T) {
	ctx, s, sid, err := login(t, "user1")
	assert.NoError(t, err)
	defer s.Close()

	ses, err := s.FindBySessionToken(ctx, sid+"_not_found")
	assert.Nil(t, ses)
	assert.ErrorIs(t, err, ErrInvalidSessionToken)
}

func TestSessionStorage_Logout(t *testing.T) {
	ctx, s, sid, err := login(t, "user1")
	assert.NoError(t, err)
	defer s.Close()

	err = s.Logout(ctx, sid)
	assert.NoError(t, err)

	ses, err := s.FindBySessionToken(ctx, sid)
	assert.Nil(t, ses)
	assert.ErrorIs(t, err, ErrInvalidSessionToken)
}

func TestSessionStorage_SingleSession_Timeout(t *testing.T) {
	ctx, s, sid, err := login(t, "user1")
	assert.NoError(t, err)
	defer s.Close()

	now := currentTime(ctx)

	// Idle Timeout
	afterIdleTimeout := now.Add(time.Hour * 4)
	ctx2 := setFixTime(context.Background(), afterIdleTimeout)

	ses, err := s.FindBySessionToken(ctx2, sid)
	assert.NotNil(t, ses)
	assert.NoError(t, err)
	assert.Equal(t, "user1", ses.UserID)
	assert.Equal(t, IdleTimeoutSession, ses.Status)

	// Absolute timeout
	afterAbsoluteTimeout := now.Add(time.Hour * 40 * 24)
	ctx4 := setFixTime(context.Background(), afterAbsoluteTimeout)

	ses, err = s.FindBySessionToken(ctx4, sid)
	assert.Nil(t, ses)
	assert.ErrorIs(t, err, ErrInvalidSessionToken)
}

func TestSessionStorage_MultipleSession(t *testing.T) {
	// first login
	_, s, sid1, err := login(t, "user1")
	defer s.Close()

	assert.NoError(t, err)

	now := time.Date(2021, time.July, 2, 10, 0, 0, 0, time.Local)
	ctx := setFixTime(context.Background(), now)

	// same user login again from other browsers
	oldSessionID, err := s.StartLogin(ctx, map[string]string{"redirectURL": "/"})
	assert.NotEqual(t, "", oldSessionID)

	r := dummyRequest()
	user := dummyUser("user1")

	sid2, _, err := s.StartSession(ctx, oldSessionID, user, r)
	assert.NoError(t, err)
	assert.NotEqual(t, sid1, sid2)

	sessions, err := s.GetUserSessions(ctx, "user1")

	actual := []string{sessions[0].ID, sessions[1].ID}
	expected := []string{sid1, sid2}
	sort.Strings(actual)
	sort.Strings(expected)

	assert.Len(t, sessions, 2)
	assert.Equal(t, expected, actual)
}

func Test_parseDirective(t *testing.T) {
	type args struct {
		src string
	}
	tests := []struct {
		name    string
		args    args
		want    *Directive
		wantErr bool
	}{
		{
			name: "simple assign",
			args: args{
				src: "Wru-Set-Session-Data: key=value",
			},
			want: &Directive{
				Key:   "key",
				Value: "value",
			},
		},
		{
			name: "simple remove",
			args: args{
				src: "Wru-Set-Session-Data: key=",
			},
			want: &Directive{
				Key:   "key",
				Value: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDirective(tt.args.src)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDirective() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseDirective() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionStorage_UpdateSessionData(t *testing.T) {
	ctx, s, sid, err := login(t, "user1")
	defer s.Close()

	now := time.Date(2021, time.July, 2, 11, 30, 0, 0, time.Local)
	ctx = setFixTime(context.Background(), now)

	s.UpdateSessionData(ctx, sid, []string{
		"key=value",
	})

	ses, err := s.FindBySessionToken(ctx, sid)
	assert.NotNil(t, ses)
	assert.NoError(t, err)
	assert.Equal(t, "user1", ses.UserID)
	assert.Equal(t, ActiveSession, ses.Status)
	assert.Equal(t, "value", ses.Data["key"])

	// this is not expired because UpdateSessionData updates IdleTimeout
	now = time.Date(2021, time.July, 2, 13, 00, 0, 0, time.Local)
	ctx = setFixTime(context.Background(), now)

	ses, err = s.FindBySessionToken(ctx, sid)
	assert.NotNil(t, ses)
	assert.NoError(t, err)
	assert.Equal(t, "user1", ses.UserID)
	assert.Equal(t, ActiveSession, ses.Status)
	assert.Equal(t, "value", ses.Data["key"])
}

func TestSessionStorage_RenewSession(t *testing.T) {
	ctx, s, sid, err := login(t, "user1")
	defer s.Close()

	// Sid is active
	now := time.Date(2021, time.July, 2, 10, 30, 0, 0, time.Local)
	ctx = setFixTime(context.Background(), now)
	sid2, err := s.RenewSession(ctx, sid)
	assert.NoError(t, err)
	assert.Equal(t, sid, sid2)

	// Between IdleTimeout and AbsoluteTimeout
	now = time.Date(2021, time.July, 10, 10, 30, 0, 0, time.Local)
	ctx = setFixTime(context.Background(), now)
	sid3, err := s.RenewSession(ctx, sid)
	assert.NoError(t, err)
	assert.NotEqual(t, sid, sid3)

	// Old sid is expired
	ses, err := s.FindBySessionToken(ctx, sid)
	assert.Nil(t, ses)
	assert.Error(t, err)

	// renewed sid is active
	ses2, err := s.FindBySessionToken(ctx, sid3)
	assert.NotNil(t, ses2)
	assert.NoError(t, err)
}

