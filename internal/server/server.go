package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Kcchouette/gowlarr/internal/search"
	"github.com/Kcchouette/gowlarr/internal/store"
)

type Server struct {
	engine     *search.Engine
	store      *store.Store
	apiKey     string
	corsOrigin string
	addr       string
	srv        *http.Server
}

func New(addr, apiKey, corsOrigin string, engine *search.Engine, st *store.Store) *Server {
	s := &Server{
		engine:     engine,
		store:      st,
		apiKey:     apiKey,
		corsOrigin: corsOrigin,
		addr:       addr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api", s.requireAPIKey(s.handleAPI))
	mux.HandleFunc("/", s.handleRoot)

	s.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s
}

func (s *Server) ListenAndServe() error {
	slog.Info("server started", "addr", s.addr)
	slog.Info("API endpoint", "url", fmt.Sprintf("http://%s/api?t=search&q=<query>&apikey=<key>", s.addr))
	return s.srv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.applyCORS(w)
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, "Gowlarr Torznab/Newznab server\n")
	fmt.Fprint(w, "Use /api?t=search&q=<query>&apikey=<key>\n")
}

func (s *Server) applyCORS(w http.ResponseWriter) {
	if s.corsOrigin == "" {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", s.corsOrigin)
	w.Header().Set("Vary", "Origin")
}
