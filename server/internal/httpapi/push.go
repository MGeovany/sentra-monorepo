package httpapi

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/mgeovany/sentra/server/internal/auth"
	"github.com/mgeovany/sentra/server/internal/repo"
)

func pushHandler(store repo.PushStore) http.Handler {
	if store == nil {
		store = repo.DisabledPushStore{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err == nil {
			if v := r.Context().Value(ctxKeySignedBody{}); v != nil {
				if b, ok := v.([]byte); ok {
					body = b
				}
			}
		}
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := validateJSONAgainstPushSchema(body); err != nil {
			// Keep response minimal, but log the reason for debugging.
			// Never log secrets: payload is expected to be encrypted blobs.
			// (Still avoid printing the full body.)
			log.Printf("push payload rejected err=%v", err)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "invalid push payload")
			return
		}

		user, ok := auth.UserFromContext(r.Context())
		if !ok || user.ID == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "unauthorized")
			return
		}

		// Decode the already-validated payload so we can pass it to the DB RPC.
		var payload any
		if err := json.Unmarshal(body, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		res, err := store.Push(r.Context(), user.ID, payload)
		if err != nil {
			log.Printf("push store failed user_id=%s err=%v", user.ID, err)
			switch err {
			case repo.ErrDBNotConfigured:
				writeHTTPError(w, http.StatusServiceUnavailable, "db not configured", err)
			default:
				writeHTTPError(w, http.StatusInternalServerError, "push failed", err)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(res)
	})
}
