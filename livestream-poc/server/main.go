// Command server is the Go backend for the live-stream + comments MVP: it
// mints LiveKit tokens and runs the SSE comment gateway. LiveKit itself
// handles the actual video/audio SFU — see docker-compose.yml.
package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"livestream-poc-server/internal/comments"
	"livestream-poc-server/internal/config"
	"livestream-poc-server/internal/token"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	hub := comments.NewHub()
	svc := comments.NewService(hub)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors(cfg.CORSAllowOrigin))

	r.Options("/*", func(w http.ResponseWriter, r *http.Request) {})
	r.Handle("/token", token.Handler(cfg.LiveKitAPIKey, cfg.LiveKitSecret))
	r.Post("/comments", comments.PostComment(svc))
	r.Get("/stream", comments.Stream(svc))

	addr := ":" + cfg.HTTPPort
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// cors is a minimal CORS layer: the React dev server and the Go API run on
// different origins, and both fetch() (POST /comments) and EventSource
// (GET /stream) need Access-Control-Allow-Origin to work cross-origin.
func cors(allowOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			next.ServeHTTP(w, r)
		})
	}
}
