package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/mgeovany/sentra/server/internal/auth"
	"github.com/mgeovany/sentra/server/internal/repo"
)

type vaultKeyEnvelopeV1 struct {
	V int `json:"v"`
}

func vaultKeyHandler(store repo.VaultKeyStore) http.Handler {
	if store == nil {
		store = repo.DisabledVaultKeyStore{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.UserFromContext(r.Context())
		if !ok || strings.TrimSpace(user.ID) == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			doc, ok, err := store.Get(r.Context(), user.ID)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, "vault key get failed")
				return
			}
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, "not found")
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(doc)
			return
		case http.MethodPut:
			r.Body = http.MaxBytesReader(w, r.Body, 32<<10) // 32 KiB
			body, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var env vaultKeyEnvelopeV1
			if err := json.Unmarshal(body, &env); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if env.V != 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, "invalid envelope version")
				return
			}
			if err := store.Upsert(r.Context(), user.ID, body); err != nil {
				switch err {
				case repo.ErrDBNotConfigured:
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = io.WriteString(w, "db not configured")
				case repo.ErrDBMisconfigured:
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = io.WriteString(w, "db misconfigured")
				default:
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = io.WriteString(w, "vault key save failed")
				}
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "ok")
			return
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})
}
