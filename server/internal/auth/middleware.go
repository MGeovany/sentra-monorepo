package auth

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
)

type contextKey string

const userKey contextKey = "sentra.user"

type Middleware struct {
	verifier Verifier
}

func NewMiddleware(verifier Verifier) Middleware {
	if verifier == nil {
		verifier = DisabledVerifier{}
	}
	return Middleware{verifier: verifier}
}

func (m Middleware) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			log.Printf("auth missing token method=%s path=%s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "unauthorized")
			return
		}

		u, err := m.verifier.Verify(token)
		if err != nil {
			if errors.Is(err, ErrAuthNotConfigured) {
				log.Printf("auth not configured method=%s path=%s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, "auth not configured")
				return
			}

			log.Printf("auth verify failed method=%s path=%s err=%v", r.Method, r.URL.Path, err)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "unauthorized")
			return
		}

		ctx := context.WithValue(r.Context(), userKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func UserFromContext(ctx context.Context) (User, bool) {
	v := ctx.Value(userKey)
	u, ok := v.(User)
	return u, ok
}

func bearerToken(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(v, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(v, prefix))
}
