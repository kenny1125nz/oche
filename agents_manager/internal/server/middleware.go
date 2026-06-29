package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/anomalyco/agents_manager/internal/auth"
)

type contextKey string

const claimsKey contextKey = "claims"

var staticExtensions = []string{".html", ".css", ".js", ".json", ".png", ".svg", ".ico", ".woff2"}

func isStaticFile(path string) bool {
	if path == "/" || path == "/index.html" {
		return true
	}
	for _, ext := range staticExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func authMiddleware(authManager *auth.Manager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isStaticFile(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/auth/") {
			next.ServeHTTP(w, r)
			return
		}

		token := ""
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			token = strings.TrimPrefix(h, "Bearer ")
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		}

		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "missing token"})
			return
		}

		claims, err := authManager.ValidateToken(token)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid token"})
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func setupCheckMiddleware(authManager *auth.Manager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isStaticFile(r.URL.Path) || strings.HasPrefix(r.URL.Path, "/auth/") {
			next.ServeHTTP(w, r)
			return
		}

		if !authManager.PasswordStore().IsSet() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "password not set, use POST /auth/setup"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
