package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"
)

var htmlTemplate = template.Must(template.New("html").Parse(`
<!DOCTYPE html>
<html lang="ja">
<head>
    <meta charset="UTF-8">
    <title>{{.RequestURL}}</title>
</head>
<body>
<h1>WRU sample backend server</h1>
<h2>Login Status</h2>
<p>RequestURL: {{.RequestURL}}</h1>
<p>UserID: {{.UserID}}</p>
<p>Last Access At: {{.LastAccessAt.Format "2006/1/2 15:04:05"}}</p>
<p>Login At: {{.LoginAt.Format "2006/1/2 15:04:05"}}</p>
<h2>Session Storage Sample</h2>
<p>Access Count: {{.AccessCount}} <form action="/increment" method="post"><input type="submit" value="Increment" /></form></p>
<h2>Links</h2>
<p><a href="/.wru/logout">Logout</a></p>
<p><a href="/.wru/user">User Status</a></p>
<p><a href="/.wru/user/sessions">Session Information</a></p>
</body>
</html>
`))

type PageContext struct {
	RequestURL   string
	UserID       string
	LastAccessAt time.Time
	LoginAt      time.Time
	AccessCount  int
}

type Session struct {
	LoginAt      int64             `json:"login_at"`
	ExpireAt     int64             `json:"expire_at"`
	LastAccessAt int64             `json:"last_access_at"`
	UserID       string            `json:"id"`
	DisplayName  string            `json:"name"`
	Email        string            `json:"email"`
	Organization string            `json:"org"`
	Scopes       []string          `json:"scopes"`
	Data         map[string]string `json:"data"`
}

func main() {
	fmt.Println("test server is running at :8080")
	fmt.Println("Run wru with the following env var:")
	fmt.Println(`  WRU_FORWARD_TO: "/ => http://localhost:8080"`)

	var headerKey string
	if key, ok := os.LookupEnv("WRU_SERVER_SESSION_FIELD"); ok {
		headerKey = key
	} else {
		headerKey = "Wru-Session"
	}
	fmt.Printf("This test server checks %s header field\n", headerKey)

	http.ListenAndServe(":8080", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("🪓 ", r.RequestURI)

		c := &PageContext{
			RequestURL: r.RequestURI,
		}

		h := r.Header.Get(headerKey)
		if h != "" {
			var s Session
			json.NewDecoder(strings.NewReader(h)).Decode(&s)
			c.UserID = s.UserID
			c.LoginAt = time.Unix(0, s.LoginAt)
			c.LastAccessAt = time.Unix(0, s.LastAccessAt)
			count, err := strconv.ParseInt(s.Data["access-count"], 10, 64)
			if err == nil {
				c.AccessCount = int(count)
			}
		}
		if r.Method == "POST" && r.RequestURI == "/increment" {
			w.Header().Set("Wru-Set-Session-Data", fmt.Sprintf("access-count=%d", c.AccessCount + 1))
		}
		htmlTemplate.Execute(w, c)
	}))
}
