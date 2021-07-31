package wru

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/future-architect/gocloudurls"
	"github.com/gookit/color"
	"gocloud.dev/blob"
)

type IDPlatform string

const (
	Twitter IDPlatform = "Twitter"
	GitHub  IDPlatform = "GitHub"
	OIDC    IDPlatform = "OIDC"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrNotModified  = errors.New("not modified")
)

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

type IdentityRegister struct {
	fromID         map[string]*User
	fromIDPUser    map[IDPlatform]map[string]*User
	sourceBlobUrl  string
	fileModifiedAt time.Time
	lock           *sync.RWMutex
}

func (ir IdentityRegister) AllUsers() []*User {
	ir.lock.RLock()
	defer ir.lock.RUnlock()
	result := make([]*User, 0, len(ir.fromID))
	for _, u := range ir.fromID {
		result = append(result, u)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UserID < result[j].UserID
	})
	return result
}

func (ir *IdentityRegister) FindUserByID(userID string) (*User, error) {
	ir.lock.RLock()
	defer ir.lock.RUnlock()
	u, ok := ir.fromID[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	return u, nil
}

func (ir *IdentityRegister) FindUserOf(idp IDPlatform, userID string) (*User, error) {
	ir.lock.RLock()
	defer ir.lock.RUnlock()
	if idpUsers, ok := ir.fromIDPUser[idp]; ok {
		if u, ok := idpUsers[userID]; ok {
			return u, nil
		}
	}
	return nil, ErrUserNotFound
}

var userRE = regexp.MustCompile(`WRU_USER_\d+=(.*)`)

func NewIdentityRegister(ctx context.Context, c *Config, out io.Writer) (*IdentityRegister, []string, error) {
	if c != nil && c.UserTable != "" {
		return NewIdentityRegisterFromConfig(ctx, c, out)
	}
	return NewIdentityRegisterFromEnv(ctx, os.Environ(), out)
}

func NewIdentityRegisterFromConfig(ctx context.Context, c *Config, out io.Writer) (*IdentityRegister, []string, error) {
	ir := &IdentityRegister{
		fromID:      make(map[string]*User),
		fromIDPUser: map[IDPlatform]map[string]*User{},
		lock:        &sync.RWMutex{},
	}
	var warnings []string
	if !strings.HasPrefix(c.UserTable, ".") && !strings.HasPrefix(c.UserTable, "/") {
		var err error
		c.UserTable, err = gocloudurls.NormalizeBlobURL(c.UserTable, os.Environ())
		if err != nil {
			return nil, nil, err
		}
	}
	ir.sourceBlobUrl = c.UserTable
	users, modTime, err := readUsersFromBlob(ctx, c.UserTable, ir.fileModifiedAt)
	if err != nil {
		return nil, nil, err
	}
	ir.fileModifiedAt = modTime
	for _, u := range users {
		ir.appendUser(u)
	}
	if out != nil {
		color.Fprintf(out, "Read %d users from %s\n", len(users), c.UserTable)
	}
	if c.UserTableReloadTerm > time.Second {
		go func() {
			t := time.NewTicker(c.UserTableReloadTerm)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					ir2 := &IdentityRegister{
						fromID:      make(map[string]*User),
						fromIDPUser: map[IDPlatform]map[string]*User{},
					}
					users, modTime, err := readUsersFromBlob(ctx, c.UserTable, ir.fileModifiedAt)
					if err != nil {
						if !errors.Is(err, ErrNotModified) {
							if out != nil {
								color.Fprintf(out, "<error>Reload user table error: %s</>\n", err.Error())
							}
							return
						} else {
							// log.Println("file is not modified")
							continue
						}
					}
					for _, u := range users {
						ir2.appendUser(u)
					}
					color.Fprintf(out, "Read %d users from %s\n", len(users), c.UserTable)
					ir.lock.Lock()
					ir.fromID = ir2.fromID
					ir.fromIDPUser = ir2.fromIDPUser
					ir.fileModifiedAt = modTime
					ir.lock.Unlock()
				}
			}
		}()
	}
	return ir, warnings, nil
}

func NewIdentityRegisterFromEnv(ctx context.Context, envs []string, out io.Writer) (*IdentityRegister, []string, error) {
	ir := &IdentityRegister{
		fromID:      make(map[string]*User),
		fromIDPUser: map[IDPlatform]map[string]*User{},
		lock:        &sync.RWMutex{},
	}
	var warnings []string

	if out != nil {
		color.Fprintf(out, "<blue>Users (for DevMode):</>\n")
	}

	for _, env := range envs {
		u := parseUserFromEnv(env)
		if u != nil {
			ir.appendUser(u)
			if out != nil {
				color.Fprintf(out, "  '%s'(%s) @ %s (scopes: %s)\n", u.DisplayName, u.UserID, u.Organization, strings.Join(u.Scopes, ", "))
			}
		}
	}
	return ir, warnings, nil
}

func (ir *IdentityRegister) appendUser(u *User) {
	ir.fromID[u.UserID] = u
	for _, service := range u.FederatedUserAccounts {
		if service.Service != "" {
			if _, ok := ir.fromIDPUser[service.Service]; !ok {
				ir.fromIDPUser[service.Service] = make(map[string]*User)
			}
			ir.fromIDPUser[service.Service][service.Account] = u
		}
	}
}

func SplitBlobPath(resourceUrl string) (string, string, error) {
	u, err := url.Parse(resourceUrl)
	if err != nil {
		return "", "", err
	}
	if u.Scheme != "file" {
		resourcePath := u.Path
		u.Path = ""
		return u.String(), resourcePath, nil
	} else {
		dir := path.Dir(u.Path)
		base := path.Base(u.Path)
		u.Path = dir
		return u.String(), base, nil
	}
}

func readUsersFromBlob(ctx context.Context, path string, modifiedAt time.Time) ([]*User, time.Time, error) {
	if !strings.HasPrefix(path, ".") && !strings.HasPrefix(path, "/") {
		bucketUrl, res, err := SplitBlobPath(path)
		if err != nil {
			return nil, time.Time{}, err
		}
		b, err := blob.OpenBucket(ctx, bucketUrl)
		if err != nil {
			return nil, time.Time{}, err
		}
		defer b.Close()

		a, err := b.Attributes(ctx, res)
		if err != nil {
			return nil, time.Time{}, err
		}
		if modifiedAt.Before(a.ModTime) {
			r, err := b.NewReader(ctx, res, &blob.ReaderOptions{})
			if err != nil {
				return nil, time.Time{}, err
			}
			defer r.Close()
			users, err := parseUsersFromBlob(r)
			if err != nil {
				return nil, time.Time{}, err
			}
			return users, a.ModTime, err
		}
	} else {
		s, err := os.Stat(path)
		if err != nil {
			return nil, time.Time{}, err
		}
		if modifiedAt.Before(s.ModTime()) {
			f, err := os.Open(path)
			if err != nil {
				return nil, time.Time{}, err
			}
			defer f.Close()
			users, err := parseUsersFromBlob(f)
			if err != nil {
				return nil, time.Time{}, err
			}
			return users, s.ModTime(), nil
		}
	}
	return nil, time.Time{}, ErrNotModified
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
		} else if h == "github" {
			keys[i] = "github"
		} else if h == "oidc" {
			keys[i] = "oidc"
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
				case "oidc":
					u.FederatedUserAccounts = append(u.FederatedUserAccounts, FederatedAccount{
						Service: OIDC,
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
		case "oidc":
			u.FederatedUserAccounts = append(u.FederatedUserAccounts, FederatedAccount{
				Service: OIDC,
				Account: elems[1],
			})
		}
	}
	if u.UserID != "" {
		return u
	}
	return nil
}
