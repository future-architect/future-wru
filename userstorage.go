package wru

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	// "github.com/future-architect/gocloudurls"

	"github.com/gookit/color"
)

type IDPlatform string

const (
	Twitter IDPlatform = "Twitter"
	GitHub  IDPlatform = "GitHub"
)

var ErrUserNotFound = errors.New("user not found")

type FederatedAccount struct {
	Service IDPlatform `json:"service"`
	Account string     `json:"account"`
}

type User struct {
	DisplayName           string             `json:"display_name"`
	Organization          string             `json:"organization"`
	UserID                string             `json:"user_id"`
	Email                 string             `json:"email"`
	Scopes                []string           `json:"scopes"`
	FederatedUserAccounts []FederatedAccount `json:"federated_accounts"`
}

func (u User) ScopeString() string {
	return strings.Join(u.Scopes, ", ")
}

func (u User) WriteAsJson(w io.Writer) error {
	e := json.NewEncoder(w)
	return e.Encode(&u)
}

type UserStorage struct {
	fromID        map[string]*User
	fromIDPUser   map[IDPlatform]map[string]*User
	sourceBlobUrl string
	cacheDuration time.Duration
}

func (s UserStorage) AllUsers() []*User {
	result := make([]*User, 0, len(s.fromID))
	for _, u := range s.fromID {
		result = append(result, u)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UserID < result[j].UserID
	})
	return result
}

func (s *UserStorage) FindUserByID(userID string) (*User, error) {
	u, ok := s.fromID[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	return u, nil
}

func (s *UserStorage) FindUserOf(idp IDPlatform, userID string) (*User, error) {
	if idpUsers, ok := s.fromIDPUser[idp]; ok {
		if u, ok := idpUsers[userID]; ok {
			return u, nil
		}
	}
	return nil, ErrUserNotFound
}

var userRE = regexp.MustCompile(`WRU_USER_\d+=(.*)`)

func NewUserStorageFromEnv(ctx context.Context, out io.Writer) (*UserStorage, []string) {
	return NewUserStorage(ctx, os.Environ(), out)
}

func NewUserStorage(ctx context.Context, envs []string, out io.Writer) (*UserStorage, []string) {
	us := &UserStorage{
		fromID: make(map[string]*User),
		fromIDPUser: map[IDPlatform]map[string]*User{},
	}
	var warnings []string

	if out != nil {
		color.Fprintf(out, "<blue>Users (for DevMode):</>\n")
	}

	for _, env := range envs {
		if strings.HasPrefix(env, "WRU_USER_FILE=") {
			path := strings.TrimPrefix(env, "WRU_USER_FILE=")
			/* todo: blob support
			if !strings.HasPrefix(path, ".") && !strings.HasPrefix(path, "/") {

				var err error
				path, err = gocloudurls.NormalizeBlobURL(path, envs)
				if err != nil {
					// todo: warning
					continue
				}
			}*/
			us.sourceBlobUrl = path
			users, err := readUsersFromBlob(ctx, path)
			if err != nil {
				// todo: warning
				continue
			}
			for _, u := range users {
				us.appendUser(u)
			}
		}
		u := parseUserFromEnv(env)
		if u != nil {
			us.appendUser(u)
			if out != nil {
				color.Fprintf(out, "  (User) '%s'(%s) @ %s (scopes: %s)\n", u.DisplayName, u.UserID, u.Organization, strings.Join(u.Scopes, ", "))
			}
		}
	}
	return us, warnings
}

func (us *UserStorage) appendUser(u *User) {
	us.fromID[u.UserID] = u
	for _, service := range u.FederatedUserAccounts {
		if service.Service != "" {
			if _, ok := us.fromIDPUser[service.Service]; !ok {
				us.fromIDPUser[service.Service] = make(map[string]*User)
			}
			us.fromIDPUser[service.Service][service.Account] = u
		}
	}
}

func readUsersFromBlob(ctx context.Context, path string) ([]*User, error) {
	/*
		todo: blob support
		if !strings.HasPrefix(path, ".") && !strings.HasPrefix(path, "/") {

		}*/
	f, err := os.Open(path)
	if err != nil {
		return nil, err

	}
	defer f.Close()
	return parseUsersFromBlob(f)
}

func parseUsersFromBlob(r io.Reader) ([]*User, error) {
	cr := csv.NewReader(r)
	var result []*User
	headers, err := cr.Read()
	if err != nil {
		return nil, err
	}
	keys := map[int]string{}
	foundID := false
	for i, h := range headers {
		if h == "id" || h == "userid" {
			keys[i] = "id"
			foundID = true
		} else if h == "name" {
			keys[i] = "name"
		} else if h == "mail" || h == "email" {
			keys[i] = "mail"
		} else if h == "org" || h == "organization" {
			keys[i] = "org"
		} else if h == "scopes" || h == "scope" {
			keys[i] = "scope"
		} else if h == "twitter" {
			keys[i] = "twitter"
		}
	}
	if !foundID {
		return nil, errors.New("invalid csv: no id field")
	}
	for {
		records, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		u := &User{}
		for i, r := range records {
			if key, ok := keys[i]; ok {
				switch key {
				case "id":
					u.UserID = r
				case "name":
					u.DisplayName = r
				case "mail":
					u.Email = r
				case "org":
					u.Organization = r
				case "scope":
					u.Scopes = strings.Split(r, ",")
				case "twitter":
					u.FederatedUserAccounts = append(u.FederatedUserAccounts, FederatedAccount{
						Service: Twitter,
						Account: r,
					})
				case "github":
					u.FederatedUserAccounts = append(u.FederatedUserAccounts, FederatedAccount{
						Service: GitHub,
						Account: r,
					})
				}
			}
		}
		if u.UserID != "" {
			result = append(result, u)
		}
	}
	return result, nil
}

func parseUserFromEnv(env string) *User {
	match := userRE.FindStringSubmatch(env)
	if match == nil {
		return nil
	}
	u := &User{}
	for _, f := range strings.Split(match[1], ",") {
		elems := strings.SplitN(f, ":", 2)
		if len(elems) == 0 {
			// todo: warning
			continue
		} else if len(elems) == 1 {
			// todo: warning
			continue
		}
		switch elems[0] {
		case "userid":
			fallthrough
		case "id":
			u.UserID = elems[1]
		case "name":
			u.DisplayName = elems[1]
		case "mail":
			fallthrough
		case "email":
			u.Email = elems[1]
		case "org":
			fallthrough
		case "organization":
			u.Organization = elems[1]
		case "scope":
			u.Scopes = append(u.Scopes, elems[1])
		case "twitter":
			u.FederatedUserAccounts = append(u.FederatedUserAccounts, FederatedAccount{
				Service: Twitter,
				Account: elems[1],
			})
		case "github":
			u.FederatedUserAccounts = append(u.FederatedUserAccounts, FederatedAccount{
				Service: GitHub,
				Account: elems[1],
			})
		}
	}
	if u.UserID != "" {
		return u
	}
	return nil
}
