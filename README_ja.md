# WRU: who are you

![wru logo](doc/logo-small.png)

WRUはエンタープライズ向けのIdentity-awareリバースプロキシ/ミドルウェアです。
WRUは開発環境と本番環境でシームレス使える認可機構を提供します。

## WRUは何向けか？

* 2Bなユーザー向け
  * ユーザー情報はストレージ上のCSVファイル（ローカル、AWS nS3、GCP Cloud Storage）から読み込み可能

* 2Cユーザー向けではない
  * 動的なユーザー登録機構は未実装

* テスト環境向け
  * ユーザー情報はファイル以外にも環境変数で追加可能なのでDockerでも簡単に初期化可能
  * テスト環境ではパスワード不要でのログインが可能（E2Eテストがかんたん）

* 本番環境向け
  * OpenID Connectや、いくつかのSNS（現在はTwitterとGitHub）での認証をサポート

## リバースプロキシとして利用

### 導入方法

"go get"でwruのインストールが可能です:

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

`wru` コマンドはコマンドラインオプションをサポートしていません。動作のカスタマイズは環境変数を使って行います。

### WRUの実行モード

WRUは2つの実行モードを持ちます。バックエンドのサービス側はモードの違いを意識する必要はなく、常に必要な認証が行われる前提でアプリケーションを実装できます。

#### ローカル開発向けモード

![dev-mode](doc/dialogs-dev-mode.png)

サンプルの設定:

* https://localhost:8000で起動
* wruがTLSをサポート
* バックエンドサーバーはhttp://localhost:8080で稼働
* テストユーザーは人
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

#### プロダクションモード

![production-mode](doc/dialogs-production-mode.png)

サンプルの設定:

* example.comで起動（ローカルのポートは8000)
* wruではTLS提供はなし (AWS ALBなどが提供)
* バックエンドサーバーはhttp://server.example.comで稼働
* ユーザー情報はS3から読み込み（1時間ごとにリロード）
* セッションストレージはDynamoDB
* Twitter/GitHub/OpenID Connectログインを利用

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

### フロントエンド向けのエンドポイント

* `/.wru/login`: ログインページ
* `/.wru/logout`: ログアウトページ（GETアクセスでログアウト実行）
* `/.wru/user`: ユーザーページ（HTML/JSON形式をサポート）
* `/.wru/user/sessions`: ユーザーのログインセッション情報ページ（HTML/JSON形式をサポート）

### セッションストレージ

ブラウザのクッキーと似た、セッションストレージ機構を提供しています。

![session storage](doc/dialogs-session-storage.png)

ウェブアプリケーションではクッキーのように`Wru-Set-Session-Data`ヘッダーフィールドにセッションストレージに入れたいデータを入れてレスポンスとして送ります:

```http
Wru-Set-Session-Data: access-count=10
```

wruがこのヘッダーフィールドをフィルターして（ブラウザ側に送られることはありません）、その内容をセッションストレージに格納します。

格納した内容は、次回のリクエスト時に`Wru-Session`ヘッダーフィールド（`WRU_SERVER_SESSION_FIELD`環境変数で変更可能）に入れられてバックエンドサーバーに送られます。

```go
Wru-Session: {"login_at":1212121,"id":"shibu","name":"Yoshiki Shibukawa","scopes":["user","admin"],data:{"access-count":"10"}}
```

このすべての内容をGoで読むには、次の構造体を使ってパースします:

```go
type Session struct {
	LoginAt      int64             `json:"login_at"`       // ナノ秒
	ExpireAt     int64             `json:"expire_at"`      // ナノ秒
	LastAccessAt int64             `json:"last_access_at"` // ナノ秒
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



### 設定

#### wruのプロセスの関連の設定

* `PORT`: wruが利用するポート番号（デフォルトは3000)
* `HOST`: wruが外部から利用可能なホスト名（必須）。OAuth/OpenID Connectのコールバック先としても利用される。
* `WRU_DEV_MODE`: 実行モードの変更（次節で説明）
* `WRU_TLS_CERT` と `WRU_TLS_KEY`: TLSのサーバーを起動

### ストレージ設定

WRUはユーザー情報はオンメモリで持ちます。ユーザー情報は、CSVファイルや環境変数から読み込みます。

* `WRU_SESSION_STORAGE`: セッションストレージ。デフォルトはメモリ。DynamoDB、Firestore、MongoDBもサポート
* `WRU_USER_TABLE`: CSVファイルを読み込むローカルファイル/Blob(AWS S3、GCP Cloud Storage)のパス。
* `WRU_USER_TABLE_RELOAD_TERM`: ファイルリロード間隔
* `WRU_USER_%d`: 環境変数経由でユーザー追加（テスト用）

ユーザーを環境変数で追加する場合、カンマ区切りのタグ付きの値リストで情報を設定します:

```bash
WRU_USER_1="id:user1,name:test user 1,mail:user1@example.com,org:R&D,scope:admin,scope:user,scope:org:rd,twitter:user1"
```

ユーザー情報CSVファイルは特定のキーのヘッダー行を付与します（順序は変更可能）。

```csv
id,name,mail,org,scopes,twitter,github,oidc
user1,test user,user1@example.com,R&D,"admin,user,org:rd",user1,user1,user1@example.com
```

### バックエンドサーバー関連の設定

* `WRU_FORWARD_TO`: バックエンドサーバーを指定（必須）
* `WRU_SERVER_SESSION_FIELD`: バックエンドサーバー向けのリクエストに付与する、セッション情報のヘッダーフィールド名（デフォルトは "Wru-Session"）

#### フロントエンドのユーザー体験に関する設定

* `WRU_DEFAULT_LANDING_PAGE`: WRUはなるべく初回アクセスのあったページにログイン後に復帰させようとします。この変数はその情報が得られなかった時のデフォルトのパスです（デフォルトは'/'）
* `WRU_LOGIN_TIMEOUT_TERM`: ログイン前のセッショントークンが期限切れになる期間（デフォルトは'10m'）
* `WRU_SESSION_IDLE_TIMEOUT_TERM`: アクティブなセッショントークンがタイムアウトする期間（デフォルトは'1h'）
* `WRU_SESSION_ABSOLUTE_TIMEOUT_TERM`: セッションが最終的にタイムアウトになる期間（デフォルトは'720h'）
* `WRU_HTML_TEMPLATE_FOLDER`: ログインやユーザーページのテンプレート（デフォルトは内蔵テンプレートを利用）

#### IDプロバイダの設定

IDプロバイダを追加するには次の環境変数の設定が必要です。
コールバックアドレスはどのプロバイダーでも``${HOST}/.wru/callback``となります。
IDプロバイダ側にwruをRPとして登録する場合は、このURLを設定してください。

##### Twitter

* `WRU_TWITTER_CONSUMER_KEY`
* `WRU_TWITTER_CONSUMER_SECRET`

##### GitHub

* `WRU_GITHUB_CLIENT_ID`
* `WRU_GITHUB_CLIENT_SECRET`

##### OpenID Connect

* `WRU_OIDC_PROVIDER_URL`
* `WRU_OIDC_CLIENT_ID`
* `WRU_OIDC_CLIENT_SECRET`

#### 追加オプション

* `WRU_GEIIP_DATABASE`: GeoIP2/GeoLite2のファイル(.mmdb)。ユーザーの所在地をIPアドレスから推測するのに利用。

## ミドルウェアとしての利用

wruはHTTPサービスのミドルウェアとしても動作します。サンプルは`cmd/sampleapp`を参照してください。

`NewAuthorizationMiddleware()`関数が、動作に必要なHTTPハンドラ（ログインフォーム、OAuthのコールバックなどを含む）とミドルウェアを返します。
ミドルウェアを、wru自身のハンドラには適用しないようにしてください（無限ループとなります）。

`*wru.Config`は次のサンプルの作成方法（構造体を直接利用）のほか、`wru.NewConfigFromEnv()`でも作成できます。

```go
package main

import (
  "context"
  "fmt"
  "net/http"
  "os"
  "os/signal"

  "github.com/go-chi/chi/v5"
  "github.com/go-chi/chi/v5/middleware"
  "gitlab.com/osaki-lab/wru"

  // Select backend of session storage (docstore) and identity register (blob)
  _ "gocloud.dev/docstore/memdocstore"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	r := chi.NewRouter()
	c := &wru.Config{
		Port:    3000,
		Host:    "http://localhost:3000",
		DevMode: true,
		Users: []*wru.User{
			{
				DisplayName:  "Test User 1",
				Organization: "Secret",
				UserID:       "testuser01",
				Email:        "testuser01@example.com",
			},
		},
	}
	wruHandler, authMiddleware := wru.NewAuthorizationMiddleware(ctx, c, os.Stdout)
	r.Use(middleware.Logger)
	r.Mount("/", wruHandler)
	r.With(authMiddleware).Get("/", func(w http.ResponseWriter, r *http.Request) {
		_, session := wru.GetSession(r)
		w.Write([]byte("welcome " + session.UserID))
	})
    http.ListenAndServe(fmt.Sprintf(":%d", c.Port), r)
}
```

## ライセンス

Apache 2