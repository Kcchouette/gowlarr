package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Kcchouette/gowlarr/internal/search"
	"github.com/Kcchouette/gowlarr/internal/store"
)

type Server struct {
	engine *search.Engine
	store  *store.Store
	apiKey string
	addr   string
	srv    *http.Server
}

func New(addr, apiKey string, engine *search.Engine, st *store.Store) *Server {
	s := &Server{
		engine: engine,
		store:  st,
		apiKey: apiKey,
		addr:   addr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api", s.requireAPIKey(s.handleAPI))
	mux.HandleFunc("/", s.handleRoot)

	s.srv = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	return s
}

func (s *Server) ListenAndServe() error {
	fmt.Printf("Gowlarr server listening on %s\n", s.addr)
	fmt.Printf("API endpoint: http://%s/api?t=search&q=<query>&apikey=<key>\n", s.addr)
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
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, "Gowlarr Torznab/Newznab server\n")
	fmt.Fprint(w, "Use /api?t=search&q=<query>&apikey=<key>\n")
}
