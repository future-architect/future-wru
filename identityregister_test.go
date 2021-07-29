package wru

import (
	"context"
	"io"
	"time"

	// _ "gocloud.dev/blob/fileblob"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLocalUserStorage(t *testing.T) {
	type args struct {
		envs []string
		requestUserID string
	}
	tests := []struct {
		name string
		args args
		want *User
		wantErr error
	}{
		{
			name: "init over env var: empty",
			args: args{
				envs: []string{
				},
				requestUserID: "user1",
			},
			wantErr: ErrUserNotFound,
		},
		{
			name: "init over env var: found",
			args: args{
				envs: []string{
					`WRU_USER_1=id:user1,name:test user,mail:user1@example.com,org:R&D,scope:admin,scope:user,scope:org:rd,twitter:user1`,
				},
				requestUserID: "user1",
			},
			want: &User{
				DisplayName:          "test user",
				Organization:         "R&D",
				UserID:               "user1",
				Email:                "user1@example.com",
				Scopes:               []string{"admin", "user", "org:rd"},
				FederatedUserAccounts: []FederatedAccount{
					{
						Service: Twitter,
						Account: "user1",
					},
				},
			},
		},
		{
			name: "init over file: found",
			args: args{
				envs: []string{
					`WRU_USER_1=id:user1,name:test user,mail:user1@example.com,org:R&D,scope:admin,scope:user,scope:org:rd,twitter:user1`,
				},
				requestUserID: "user1",
			},
			want: &User{
				DisplayName:          "test user",
				Organization:         "R&D",
				UserID:               "user1",
				Email:                "user1@example.com",
				Scopes:               []string{"admin", "user", "org:rd"},
				FederatedUserAccounts: []FederatedAccount{
					{
						Service: Twitter,
						Account: "user1",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _, _ := NewIdentityRegisterFromEnv(context.Background(), tt.args.envs, io.Discard)
			u, err := s.FindUserByID(tt.args.requestUserID)
			if tt.wantErr == nil {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, u)
			} else {
				assert.Equal(t, err, tt.wantErr)
			}
		})
	}
}

func Test_parseUsersFromBlob(t *testing.T) {
	type args struct {
		src string
	}
	tests := []struct {
		name    string
		args    args
		want    []*User
		wantErr bool
	}{
		{
			name: "no user",
			args: args{
				src: `id,name,org,scopes,email
`,
			},
			want: nil,
			wantErr: false,
		},
		{
			name: "no id",
			args: args{
				src: `name,org,scopes,email
`,
			},
			want: nil,
			wantErr: true,
		},
		{
			name: "one user",
			args: args{
				src: `id,name,mail,org,scopes,twitter
user1,test user,user1@example.com,R&D,"admin,user,org:rd",user1
`,
			},
			want: []*User{
				{
					DisplayName:          "test user",
					Organization:         "R&D",
					UserID:               "user1",
					Email:                "user1@example.com",
					Scopes:               []string{"admin", "user", "org:rd"},
					FederatedUserAccounts: []FederatedAccount{
						{
							Service: Twitter,
							Account: "user1",
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUsersFromBlob(strings.NewReader(tt.args.src))
			if (err != nil) != tt.wantErr {
				t.Errorf("parseUsersFromBlob() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseUsersFromBlob() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_readUsersFromBlob(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		args    args
		want    []*User
		wantErr bool
	}{
		{
			name: "from path: error",
			args: args{
				path: "./testdata/testuser.notfound.csv",
			},
			wantErr: true,
		},
		{
			name: "from path: success",
			args: args{
				path: "./testdata/testuser_for_ut.csv",
			},
			want: []*User{
				{
					DisplayName:          "test user1",
					Organization:         "R&D",
					UserID:               "testuser1",
					Email:                "testuser1@example.com",
					Scopes:               []string{"admin", "user", "org:rd"},
					FederatedUserAccounts: []FederatedAccount{
						{
							Service: Twitter,
							Account: "testuser1",
						},
						{
							Service: GitHub,
							Account: "testuser1",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "from file scheme: error",
			args: args{
				path: "file://./testdata/testuser.notfound.csv",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := readUsersFromBlob(context.Background(), tt.args.path, time.Time{})
			if (err != nil) != tt.wantErr {
				t.Errorf("readUsersFromBlob() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUserStorage_FindUserAPIs(t *testing.T) {
	envs := []string{
		`WRU_USER_1=id:user1,name:test user,mail:user1@example.com,org:R&D,scope:admin,scope:user,scope:org:rd,twitter:user1`,
	}
	us, warnings, _ := NewIdentityRegisterFromEnv(context.Background(), envs, io.Discard)
	assert.Nil(t, warnings)
	u, err := us.FindUserOf(Twitter, "user1")
	assert.NoError(t, err)
	assert.Equal(t, "user1@example.com", u.Email)
}