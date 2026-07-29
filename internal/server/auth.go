package server

import (
	"crypto/subtle"
	"net/http"
)

func (s *Server) requireAPIKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.applyCORS(w)
		if s.apiKey == "" {
			next(w, r)
			return
		}

		// Le protocole Torznab/Newznab transmet classiquement l'API key en
		// query string (`apikey=`) : c'est la convention interop attendue par
		// Prowlarr, Jackett et les clients *arr, pas un choix spécifique à
		// Gowlarr.
		apikey := r.URL.Query().Get("apikey")
		if subtle.ConstantTimeCompare([]byte(apikey), []byte(s.apiKey)) != 1 {
			http.Error(w, "Invalid API key", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}
