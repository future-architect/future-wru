package wru

import (
	"github.com/stretchr/testify/assert"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("🦀 ")
}

func TestNewProxy(t *testing.T) {
	var lastCalled string
	server1 := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		lastCalled = "server1"
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		lastCalled = "server2"
	}))
	defer server2.Close()
	p, err := NewReverseProxy(&Config{
		ForwardTo: []Route{
			{
				Host: mustParseUrl(server1.URL),
				Path: "/server1/",
			},
			{
				Host: mustParseUrl(server2.URL),
				Path: "/server2/",
			},
		},
	}, nil)
	assert.NoError(t, err)
	if err != nil {
		return
	}

	proxy := httptest.NewServer(p)
	type args struct {
		path string
	}
	tests := []struct {
		name           string
		args           args
		wantStatus     int
		wantLastCalled string
	}{
		{
			name: "success 1",
			args: args{
				path: "/server1/test",
			},
			wantStatus:     200,
			wantLastCalled: "server1",
		},
		{
			name: "success 2",
			args: args{
				path: "/server2/test",
			},
			wantStatus:     200,
			wantLastCalled: "server2",
		},
		{
			name: "missing",
			args: args{
				path: "/server3/test",
			},
			wantStatus:     404,
			wantLastCalled: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lastCalled = ""
			res1, err := http.Get(proxy.URL + tt.args.path)
			assert.NoError(t, err)
			if err != nil {
				return
			}
			assert.Equal(t, tt.wantStatus, res1.StatusCode)
			assert.Equal(t, tt.wantLastCalled, lastCalled)
		})
	}
}
