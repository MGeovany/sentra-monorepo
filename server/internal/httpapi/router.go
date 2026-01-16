package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/mgeovany/sentra/server/internal/auth"
)

type Deps struct {
	Auth auth.Middleware
}

func New(deps Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})

	mux.Handle("/users/me", deps.Auth.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := auth.UserFromContext(r.Context())
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(user)
	})))

	return mux
}
