# WRU: who are you

![wru logo](doc/logo-small.png)

WRU is an identity aware proxy for enterprise users.
It provides seamless authentication experience between development environemnt and production environment.

## What is WRU for

* For enterprise users
    * Easy to inject users via CSV files on storage (local, AWS S3, GCP Cloud Storage)

* Not For consumer service users
    * It is not supporting creating user

* For testing
    * Inject user information from env vars/files. You can setup via Docker easily
    * No password required (E2E test friendly)

* For production
    * It supports OpenID Connect, some SNS (Twitter and GitHub for now) to login.

## Getting Started

You can get wru by "go get".

```bash
$ go get -u gitlab.com/osaki-lab/wru/cmd/wru
$ wru
Port: 8000
TLS: enabled
Debug: false
Forward To:
  / => http://localhost:8080 ()
Twitter Login: OK
GitHub Login: OK
Users (for Debug):
  (User) 'test user 1'(user1) @ R&D (scopes: admin, user, org:rd)
  (User) 'test user 2'(user2) @ HR (scopes: user, org:hr)
starting wru server at https://localhost:8000  
```

`wru` command doesn't have command line options. You can control it via environment variables.

## WRU modes

WRU has two modes. This absorbs the difference of usecases and
your web service always get only authorized requests.

### WRU for local development

![dev-mode](doc/dialogs-dev-mode.png)

Sample configuration:

* Launch at https://localhost:8000
* Enable HTTPS by wru
* A backend server is at http://localhost:8080
* Two test users
* Session storage is in-memory mode (turn off wru resets data)

```bash
$ export WRU_DEV_MODE=true
$ export WRU_TLS_CERT="-----BEGIN CERTIFICATE-----\naaaabbbbbcccccdddd....zzzz\n-----END CERTIFICATE-----"
$ export WRU_TLS_KEY="-----BEGIN PRIVATE KEY-----\nZZZZYYYYYYXXXX.....BBBBAAAA\n-----END PRIVATE KEY-----"
$ export WRU_FORWARD_TO="/ => http://localhost:8080"
$ export WRU_USER_1="id:user1,name:test user 1,mail:user1@example.com,org:R&D,scope:admin,scope:user,scope:org:rd,twitter:user1,github:user1"
$ export WRU_USER_2="id:user2,name:test user 2,mail:user2@example.com,org:HR,scope:user,scope:org:hr,twitter:user2,github:user2"
$ PORT=8000 HOST=https://localhost:8000 wru
```

### WRU for production

![production-mode](doc/dialogs-production-mode.png)

Sample configuration:

* Launch at example.com (local port is 8000)
* No HTTPS by wru (AWS ALB does)
* A backend server is at http://server.example.com
* User information is in S3 (and reread it every hour)
* Session storage is in DynamoDB
* Twitter/GitHub/OpenID Connect login is available

```bash
$ export WRU_DEV_MODE=false
$ export WRU_FORWARD_TO="/ => http://server.example.com"
$ export WRU_USER_TABLE="s3://my-app-usertable/user-list.csv?region=us-west-1"
$ export WRU_USER_TABLE_RELOAD_TERM=1h
$ export WRU_TWITTER_CONSUMER_KEY=1111111
$ export WRU_TWITTER_CONSUMER_SECRET=22222222
$ export WRU_GITHUB_CLIENT_ID=33333333
$ export WRU_GITHUB_CLIENT_SECRET=44444444
$ export WRU_OIDC_PROVIDER_URL=http://keycloak.example.com
$ export WRU_OIDC_CLIENT_ID=55555555
$ export WRU_OIDC_CLIENT_SECRET=66666666
$ PORT=8000 HOST=https://example.com wru
```

## End Points for frontend

* `/.wru/login`: Login page
* `/.wru/logout`: Logout page (it works just GET access)
* `/.wru/user`: User page (it supports HTML and JSON)
* `/.wru/user/sessions`: User session page (it supports HTML and JSON)

## Session Storage

It supports session storage feature similar to browsers' cookie.

![session storage](doc/dialogs-session-storage.png)

Your web application sends data that will be in the session storage with in `Wru-Set-Session-Data` header field in response like this:

```http
Wru-Set-Session-Data: access-count=10
```

wru filter this content (browser doesn't retrieve this header field) and store its content in session storage.
This content is added to `Wru-Session` header field (you can modify via `WRU_SERVER_SESSION_FIELD` env var) like this:

```go
Wru-Session: {"login_at":1212121,"id":"shibu","name":"Yoshiki Shibukawa","scopes":["user","admin"],data:{"access-count":"10"}}
```

To read all content of this field in Go, you can parse it via the following structure:

```go
type Session struct {
	LoginAt      int64             `json:"login_at"`       // nano-seconds
	ExpireAt     int64             `json:"expire_at"`      // nano-seconds
	LastAccessAt int64             `json:"last_access_at"` // nano-seconds
	UserID       string            `json:"id"`
	DisplayName  string            `json:"name"`
	Email        string            `json:"email"`
	Organization string            `json:"org"`
	Scopes       []string          `json:"scopes"`
	Data         map[string]string `json:"data"`
}

func ParseSession(r *http.Request) (*Session, error) {
  h := r.Header.Get("Wru-Session")
  if h != "" {
    var s Session
    err := json.NewDecoder(strings.NewReader(h)).Decode(&s)
    if err != nil {
      return nil, err
    }
    return &s, nil
  }
  return nil, err
}
```



## Configuration

### Server Configuration

* `PORT`: Port number that wru uses (default is 3000)
* `HOST`: Host name that wru is avaialble (required). It is used for callback of OAuth/OpenID Connect.
* `WRU_DEV_MODE`: Change mode (described bellow)
* `WRU_TLS_CERT` and `WRU_TLS_KEY`: Launch TLS server

## Storage Configuration

WRU stores user information on-memory. You can add user via CSV or env vars.

* `WRU_SESSION_STORAGE`: Session storage. Default is in memory. It supports DynamoDB, Firestore, MongoDB.
* `WRU_USER_TABLE`: This is local file path to read CSV (Blob support is in plan).
* `WRU_USER_TABLE_RELOAD_TERM`: Reload term (WIP)
* `WRU_USER_%d`: Add user via environment variable (for testing).

If you add user via env var, you use comma separated tag list:

```bash
WRU_USER_1="id:user1,name:test user 1,mail:user1@example.com,org:R&D,scope:admin,scope:user,scope:org:rd,twitter:user1"
```

User table CSV file should have specific header row.

```csv
id,name,mail,org,scopes,twitter,github,oidc
user1,test user,user1@example.com,R&D,"admin,user,org:rd",user1,user1,user1@example.com
```

## Backend Server Configuration

* `WRU_FORWARD_TO`: Specify you backend server (required)
* `WRU_SERVER_SESSION_FIELD`: Header field name that WRU adds to backend request (default is "Wru-Session")

### Frontend User Experience Configuration

* `WRU_DEFAULT_LANDING_PAGE`: WRU tries to redirect to referrer page after login. It is used when the path is not available (default is '/').
* `WRU_LOGIN_TIMEOUT_TERM`: Login session token's expiration term (default is '10m')
* `WRU_SESSION_IDLE_TIMEOUT_TERM`: Active session token's timeout term (default is '1h')
* `WRU_SESSION_ABSOLUTE_TIMEOUT_TERM`: Absolute session token's timeout term (default is '720h')
* `WRU_HTML_TEMPLATE_FOLDER`: Login/User pages' template (default tempalte is embedded ones)

### ID Provider Configuration

To enable ID provider connection, set the following env vars.
The callback address will be ``${HOST}/.wru/callback``. You should register the URL in the setting screen of the ID provider.

#### Twitter

* `WRU_TWITTER_CONSUMER_KEY`
* `WRU_TWITTER_CONSUMER_SECRET`

#### GitHub

* `WRU_GITHUB_CLIENT_ID`
* `WRU_GITHUB_CLIENT_SECRET`

#### OpenID Connect

* `WRU_OIDC_PROVIDER_URL`
* `WRU_OIDC_CLIENT_ID`
* `WRU_OIDC_CLIENT_SECRET`

### Extra Option

* `WRU_GEIIP_DATABASE`: GeoIP2 or GeoLite2 file (.mmdb) to detect user location from IP address


## License

Apache 2