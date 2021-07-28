package wru

import (
	"context"
	"errors"
	"fmt"
	"github.com/future-architect/gocloudurls"
	"github.com/mssola/user_agent"
	"github.com/shibukawa/uuid62"
	"gocloud.dev/docstore"
	"gocloud.dev/gcerrors"
	"io"
	"net/http"
)

type ServerlessSessionStorage struct {
	ctx    context.Context
	config *Config

	singleSessions *docstore.Collection
	userSessions   *docstore.Collection
}

func NewMemorySessionStorage(ctx context.Context, config *Config, prefix string) (*ServerlessSessionStorage, error) {
	sSesUrl := gocloudurls.MustNormalizeDocStoreURL("mem://", gocloudurls.Option{
		KeyName:    "id",
		Collection: prefix + "singlesessions",
	})
	sessions, err := docstore.OpenCollection(ctx, sSesUrl)
	if err != nil {
		return nil, err
	}
	uSesUrl := gocloudurls.MustNormalizeDocStoreURL("mem://", gocloudurls.Option{
		KeyName:    "id",
		Collection: prefix + "userSessions",
	})
	users, err := docstore.OpenCollection(ctx, uSesUrl)
	if err != nil {
		return nil, err
	}
	return &ServerlessSessionStorage{
		ctx:            ctx,
		config:         config,
		singleSessions: sessions,
		userSessions:   users,
	}, nil
}

func NewServerlessSessionStorage(ctx context.Context, config *Config, prefix string) (*ServerlessSessionStorage, error) {
	sessionUrl, err := gocloudurls.NormalizeDocStoreURL(config.SessionStorage, gocloudurls.Option{
		KeyName:    "id",
		Collection: prefix + "singleSessions",
	})
	if err != nil {
		return nil, err
	}
	sessions, err := docstore.OpenCollection(ctx, sessionUrl)
	if err != nil {
		return nil, err
	}
	usersUrl, err := gocloudurls.NormalizeDocStoreURL(config.SessionStorage, gocloudurls.Option{
		KeyName:    "id",
		Collection: prefix + "singleSessions",
	})
	if err != nil {
		return nil, err
	}
	users, err := docstore.OpenCollection(ctx, usersUrl)
	if err != nil {
		return nil, err
	}
	return &ServerlessSessionStorage{
		ctx:            ctx,
		config:         config,
		singleSessions: sessions,
		userSessions:   users,
	}, nil
}

func (s *ServerlessSessionStorage) Close() {
	s.singleSessions.Close()
	s.userSessions.Close()
}

func (s ServerlessSessionStorage) StartLogin(ctx context.Context, info map[string]string) (sessionID string, err error) {
	sid, err := s.generateNewSessionID(ctx)
	if err != nil {
		return "", err
	}

	now := currentTime(ctx)

	err = s.singleSessions.Create(ctx, &SingleSessionData{
		ID:           sid,
		UserID:       "",
		LoginAt:      now,
		LastAccessAt: now,
		LoginInfo:    info,
	})
	return sid, err
}

func (s ServerlessSessionStorage) AddLoginInfo(ctx context.Context, oldSessionID string, info map[string]string) (newSessionID string, err error) {
	sid, err := s.generateNewSessionID(ctx)
	if err != nil {
		return "", err
	}

	loginSession := &SingleSessionData{
		ID: oldSessionID,
	}

	err = s.singleSessions.Get(ctx, loginSession)
	if err != nil {
		return "", err
	}
	for k, v := range info {
		loginSession.LoginInfo[k] = v
	}
	s.singleSessions.Delete(ctx, &SingleSessionData{
		ID: oldSessionID,
	})
	loginSession.ID = sid
	s.singleSessions.Create(ctx, loginSession)
	return sid, nil
}

func (s *ServerlessSessionStorage) StartSession(ctx context.Context, oldSessionID string, user *User, r *http.Request, newLoginInfo map[string]string) (sessionID string, info map[string]string, err error) {
	loginSession := &SingleSessionData{
		ID: oldSessionID,
	}
	err = s.singleSessions.Get(ctx, loginSession)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			return "", nil, errors.New("StartSessionAndRedirect requires old session to login")
		}
	}
	s.singleSessions.Delete(ctx, &SingleSessionData{
		ID: oldSessionID,
	})

	sid, err := s.generateNewSessionID(ctx)
	if err != nil {
		return "", nil, err
	}

	ua := user_agent.New(r.Header.Get("User-Agent"))
	browser, version := ua.Browser()
	country, ip := getGeoLocation(s.config, r)
	loginInfo := map[string]string{
		"browser":  browser,
		"version":  version,
		"os":       ua.OS(),
		"platform": ua.Platform(),
		"country":  country,
		"ip":       ip,
	}
	for k, v := range newLoginInfo {
		loginInfo[k] = v
	}

	now := currentTime(ctx)
	err = s.singleSessions.Create(ctx, &SingleSessionData{
		ID:           sid,
		UserID:       user.UserID,
		LoginAt:      now,
		LastAccessAt: now,
		LoginInfo:    loginInfo,
	})
	if err != nil {
		return "", nil, fmt.Errorf("can't create session data: %w", err)
	}
	uSes := UserSession{ID: user.UserID}
	err = s.userSessions.Get(ctx, &uSes)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			err := s.userSessions.Create(ctx, &UserSession{
				ID:           user.UserID,
				Sessions:     []string{sid},
				Data:         make(map[string]string),
				DisplayName:  user.DisplayName,
				Email:        user.Email,
				Organization: user.Organization,
				Scopes:       user.Scopes,
			})
			if err != nil {
				return "", nil, fmt.Errorf("can't create session data: %w", err)
			}
		} else {
			return "", nil, err
		}
	} else {
		uSes.Sessions = append(uSes.Sessions, sid)
		err = s.userSessions.Replace(ctx, &uSes)
		if err != nil {
			return "", nil, fmt.Errorf("can't update session data: %w", err)
		}
	}
	return sid, loginSession.LoginInfo, nil
}

func (s *ServerlessSessionStorage) generateNewSessionID(ctx context.Context) (string, error) {
	var sid string
	oneSes := SingleSessionData{}
	for i := 0; i < 10; i++ {
		sid, _ = uuid62.V4()
		oneSes.ID = sid
		err := s.singleSessions.Get(ctx, &oneSes)
		if err != nil {
			if gcerrors.Code(err) == gcerrors.NotFound {
				return sid, nil
			} else {
				return "", err
			}
		}
	}
	return "", errors.New("getting new session id error")
}

func (s ServerlessSessionStorage) Logout(ctx context.Context, sessionID string) error {
	sSes := SingleSessionData{ID: sessionID}
	err := s.singleSessions.Get(ctx, &sSes)
	if err != nil {
		return nil
	}
	uSes := UserSession{ID: sSes.UserID}
	err = s.userSessions.Get(ctx, &uSes)
	if err != nil {
		return nil
	}
	var sesIDs []string
	for _, sesID := range uSes.Sessions {
		if sesID != sessionID {
			sesIDs = append(sesIDs)
		}
	}
	s.userSessions.Replace(ctx, &uSes)
	return s.singleSessions.Delete(ctx, &sSes)
}

func (s *ServerlessSessionStorage) GetUserSessions(ctx context.Context, userID string) ([]SingleSessionData, error) {
	iter := s.singleSessions.Query().Where("user_id", "=", userID).Get(ctx)
	defer iter.Stop()
	now := currentTime(ctx)
	var result []SingleSessionData
	at := s.config.SessionAbsoluteTimeoutTerm
	it := s.config.SessionIdleTimeoutTerm
	for {
		var sSes SingleSessionData
		err := iter.Next(ctx, &sSes)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		} else if now.Sub(sSes.LoginAt) < at && now.Sub(sSes.LastAccessAt) < it {
			result = append(result, sSes)
		}
	}
	return result, nil
}

func (s *ServerlessSessionStorage) FindBySessionToken(ctx context.Context, token string) (*Session, error) {
	sSes, uSes, status, err := s.readSession(ctx, token)
	if err != nil {
		return nil, err
	}

	if status == BeforeLogin {
		return &Session{
			LoginAt: UnixTime(sSes.LoginAt),
			UserID:  "",
			Data:    sSes.LoginInfo,
			Status:  status,
		}, nil
	} else {
		data := uSes.Data
		if data == nil {
			data = make(map[string]string)
		}
		return &Session{
			LoginAt:      UnixTime(sSes.LoginAt),
			ExpireAt:     UnixTime(sSes.LoginAt.Add(s.config.SessionAbsoluteTimeoutTerm)),
			LastAccessAt: UnixTime(sSes.LastAccessAt),
			UserID:       sSes.UserID,
			DisplayName:  uSes.DisplayName,
			Email:        uSes.Email,
			Organization: uSes.Organization,
			Scopes:       uSes.Scopes,
			Status:       status,
			Data:         data,
		}, nil
	}
}

func (s *ServerlessSessionStorage) readSession(ctx context.Context, token string) (*SingleSessionData, *UserSession, SessionStatus, error) {
	sSes := SingleSessionData{ID: token}
	err := s.singleSessions.Get(ctx, &sSes)
	if err != nil {
		code := gcerrors.Code(err)
		if code == gcerrors.NotFound {
			return nil, nil, 0, ErrInvalidSessionToken
		} else {
			return nil, nil, 0, err
		}
	}

	now := currentTime(ctx)

	if sSes.UserID == "" {
		if now.Sub(sSes.LoginAt) > s.config.LoginTimeoutTerm {
			s.Logout(ctx, token)
			return nil, nil, 0, ErrInvalidSessionToken
		}
		return &sSes, nil, BeforeLogin, nil
	}
	uSes := UserSession{ID: sSes.UserID}
	err = s.userSessions.Get(ctx, &uSes)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			return nil, nil, 0, errors.New("invalid user id")
		} else {
			return nil, nil, 0, err
		}
	}
	var status SessionStatus
	if now.Sub(sSes.LoginAt) > s.config.SessionAbsoluteTimeoutTerm {
		s.Logout(ctx, token)
		return nil, nil, 0, ErrInvalidSessionToken
	} else if now.Sub(sSes.LastAccessAt) > s.config.SessionIdleTimeoutTerm {
		status = IdleTimeoutSession
	} else {
		status = ActiveSession
	}
	return &sSes, &uSes, status, nil
}

func (s ServerlessSessionStorage) UpdateSessionData(ctx context.Context, sessionID string, directives []string) (err error) {
	sSes, uSes, _, err := s.readSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if len(directives) > 0 {
		for _, src := range directives {
			d, err := parseDirective(src)
			if err != nil {
				return err
			}
			if d.Value == "" {
				delete(uSes.Data, d.Key)
			} else {
				if uSes.Data == nil {
					uSes.Data = make(map[string]string)
				}
				uSes.Data[d.Key] = d.Value
			}
		}
		s.userSessions.Replace(ctx, uSes)
	}
	sSes.LastAccessAt = currentTime(ctx)
	return s.singleSessions.Replace(ctx, sSes)
}

func (s ServerlessSessionStorage) RenewSession(ctx context.Context, oldSessionID string) (newSessionID string, err error) {
	sSes := SingleSessionData{ID: oldSessionID}
	err = s.singleSessions.Get(ctx, &sSes)
	if err != nil {
		code := gcerrors.Code(err)
		if code == gcerrors.NotFound {
			return "", ErrInvalidSessionToken
		} else {
			return "", err
		}
	}
	now := currentTime(ctx)
	if now.Sub(sSes.LoginAt) > s.config.SessionAbsoluteTimeoutTerm {
		s.Logout(ctx, oldSessionID)
		return "", ErrInvalidSessionToken
	} else if now.Sub(sSes.LastAccessAt) > s.config.SessionIdleTimeoutTerm {
		newSessionID, err := s.generateNewSessionID(ctx)
		if err != nil {
			return "", err
		}
		sSes.ID = newSessionID
		err = s.singleSessions.Create(ctx, &sSes)
		if err != nil {
			return "", err
		}
		err = s.singleSessions.Delete(ctx, &SingleSessionData{ID: oldSessionID})
		if err != nil {
			return "", err
		}
		return newSessionID, nil
	} else {
		return oldSessionID, nil
	}
}

var _ SessionStorage = &ServerlessSessionStorage{}
