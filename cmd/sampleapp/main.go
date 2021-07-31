package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gitlab.com/osaki-lab/wru"

	// Select backend of session storage (docstore) and identity register (blob)
	_ "gocloud.dev/docstore/memdocstore"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

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
			{
				DisplayName:  "Test User 2",
				Organization: "Secret",
				UserID:       "testuser02",
				Email:        "testuser02@example.com",
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
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", c.Port),
		Handler: r,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Printf("starting wru server at http://localhost:%d\n", c.Port)
		err := srv.ListenAndServe()
		if err != http.ErrServerClosed {
			// unexpected error. port in use?
			fmt.Fprintf(os.Stderr, "Server Error: %v", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	wait, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(wait); err != nil {
		fmt.Fprintf(os.Stderr, "Shutdown Server Error: %v", err)
		os.Exit(1)
	}
}
