package wru

import (
	"github.com/stretchr/testify/assert"
	"net/url"
	"testing"
)

func mustParseUrl(src string) *url.URL {
	u, err := url.Parse(src)
	if err != nil {
		panic(err)
	}
	return u
}

func Test_parseForwardList(t *testing.T) {
	type args struct {
		src string
	}
	tests := []struct {
		name    string
		args    args
		want    []Route
		wantErr bool
	}{
		{
			name: "single route with role",
			args: args{
				src: "/api => http://localhost:8000 (admin, user)",
			},
			want: []Route{
				{
					Path:   "/api",
					Host:   mustParseUrl("http://localhost:8000"),
					Scopes: []string{"admin", "user"},
				},
			},
		},
		{
			name: "single route without role",
			args: args{
				src: "/api => http://localhost:8000",
			},
			want: []Route{
				{
					Path:   "/api",
					Host:   mustParseUrl("http://localhost:8000"),
					Scopes: nil,
				},
			},
		},
		{
			name: "wrong route without role",
			args: args{
				src: "/api =>",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseForwardList(tt.args.src)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_parseClientSessionHeader(t *testing.T) {
	type args struct {
		src string
	}
	tests := []struct {
		name     string
		args     args
		want     string
		wantType ClientSessionFieldType
		wantErr  bool
	}{
		{
			name: "for header",
			args: args{
				src: "SESSIONID",
			},
			want:     "SESSIONID",
			wantType: CookieField,
		},
		{
			name: "for cookie",
			args: args{
				src: "WruSession@cookie",
			},
			want:     "WruSession",
			wantType: CookieField,
		},
		{
			name: "for cookie(include JS)",
			args: args{
				src: "WruSession@cookie-with-js",
			},
			want:     "WruSession",
			wantType: CookieWithJSField,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotType, err := parseClientSessionField(tt.args.src)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tt.want, got)
				assert.Equal(t, tt.wantType, gotType)
				assert.NoError(t, err)
			}
		})
	}
}
