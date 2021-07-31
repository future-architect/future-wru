package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/gookit/color"
	"gitlab.com/osaki-lab/wru"

	_ "gocloud.dev/docstore/awsdynamodb"
	_ "gocloud.dev/docstore/gcpfirestore"
	_ "gocloud.dev/docstore/memdocstore"
	_ "gocloud.dev/docstore/mongodocstore"

	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("🦀 ")
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	c, err := wru.NewConfigFromEnv(ctx, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, color.Error.Sprintf("Parse config error: %s", err.Error()))
		os.Exit(1)
	}
	sessionStorage, err := wru.NewSessionStorage(ctx, c, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, color.Error.Sprintf("Connect session error: %s", err.Error()))
		os.Exit(1)
	}
	userStorage, warnings, err := wru.NewIdentityRegister(ctx, c, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, color.Error.Sprintf("Read user table error: %s", err.Error()))
		os.Exit(1)
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, color.Warn.Sprintf("User parse warning: %s", w))
	}
	handler, err := wru.NewIdentityAwareProxyHandler(c, sessionStorage, userStorage)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", c.Port),
		Handler: handler,
	}

	var cert tls.Certificate
	if c.TlsCert != "" && c.TlsKey != "" {
		cert, err = tls.X509KeyPair([]byte(c.TlsCert), []byte(c.TlsKey))
		if err != nil {
			fmt.Fprintln(os.Stderr, color.Error.Sprintf("TLS error: %s", err.Error()))
			return
		}
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		if c.TlsCert != "" && c.TlsKey != "" {
			color.Infof("starting wru server at https://localhost:%d\n", c.Port)
			srv.TLSConfig = &tls.Config{
				Certificates: []tls.Certificate{cert},
			}
			err = srv.ListenAndServeTLS("", "")
		} else {
			color.Infof("starting wru server at http://localhost:%d\n", c.Port)
			err = srv.ListenAndServe()
		}
		if err != http.ErrServerClosed {
			// unexpected error. port in use?
			fmt.Fprintln(os.Stderr, color.Error.Sprintf("Server Error: %v", err))
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	wait, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(wait); err != nil {
		fmt.Fprintln(os.Stderr, color.Error.Sprintf("Shutdown Server Error: %v", err))
		os.Exit(1)
	}
}
