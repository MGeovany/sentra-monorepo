package auth

import (
	"context"
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
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		u, err := m.verifier.Verify(token)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
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
