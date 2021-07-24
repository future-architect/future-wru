package wru

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
)

type ProxyTransport struct {
	c *Config
	s SessionStorage
}

func (p ProxyTransport) RoundTrip(req *http.Request) (res *http.Response, err error) {
	found := false
	for _, f := range p.c.ForwardTo {
		if strings.HasPrefix(req.URL.Path, f.Path) {
			req.URL.Host = f.Host.Host
			req.URL.Scheme = f.Host.Scheme
			found = true
			break
		}
	}
	if !found {
		r := httptest.NewRecorder()
		r.WriteHeader(http.StatusNotFound)
		r.WriteString(`{"status": "not found"}`)
		return r.Result(), nil
	}
	sid, ses := GetSessionInfo(req)
	if ses != nil {
		sjson, _ := json.Marshal(ses)
		req.Header.Set(p.c.ServerSessionField, string(sjson))
	}
	res, err = http.DefaultTransport.RoundTrip(req)
	if err != nil {
		log.Println(err)
	}
	if p.s != nil {
		p.s.UpdateSessionData(req.Context(), sid, res.Header.Values("Wru-Set-Session-Data"))
		res.Header.Del("Wru-Set-Session-Data")
	}
	return res, nil
}

func NewProxy(config *Config, s SessionStorage) (http.Handler, error) {
	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
		},
		Transport: &ProxyTransport{
			c: config,
			s: s,
		},
	}
	return rp, nil
}
