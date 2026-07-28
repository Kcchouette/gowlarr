package server

import (
	"crypto/subtle"
	"net/http"
)

func (s *Server) requireAPIKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey == "" {
			next(w, r)
			return
		}

		apikey := r.URL.Query().Get("apikey")
		if subtle.ConstantTimeCompare([]byte(apikey), []byte(s.apiKey)) != 1 {
			http.Error(w, "Invalid API key", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}
