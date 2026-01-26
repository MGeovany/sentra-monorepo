package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mgeovany/sentra/server/internal/auth"
	"github.com/mgeovany/sentra/server/internal/repo"
	"github.com/mgeovany/sentra/server/internal/validate"
)

func pushHandler(store repo.PushStore, idem repo.IdempotencyStore) http.Handler {
	if store == nil {
		store = repo.DisabledPushStore{}
	}
	if idem == nil {
		idem = repo.DisabledIdempotencyStore{}
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
			log.Printf("push payload rejected err=%q", err.Error())
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

		idemKey := strings.TrimSpace(r.Header.Get("X-Idempotency-Key"))
		const idemScope = "push"
		if idemKey != "" {
			if validate.IdempotencyKey(idemKey) != nil {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, "invalid idempotency key")
				return
			}

			created, err := idem.Create(r.Context(), user.ID, idemScope, idemKey, 24*time.Hour)
			if err != nil {
				// Fail open if idempotency storage is not configured or misbehaving.
				// Idempotency is an optimization; the push RPC must still be safe to retry.
				if !errors.Is(err, repo.ErrDBNotConfigured) {
					log.Printf("idempotency create failed user_id=%q key=%q err=%q", user.ID, idemKey, err.Error())
				}
				created = true
			}
			if !created {
				rec, found, getErr := idem.Get(r.Context(), user.ID, idemScope, idemKey)
				if getErr != nil {
					// Fail open if storage is broken.
					log.Printf("idempotency get failed user_id=%q key=%q err=%q", user.ID, idemKey, getErr.Error())
					created = true
				}
				if created {
					// Continue; we couldn't reliably enforce idempotency.
				} else {
					if getErr == nil && found && rec.Status == repo.IdempotencyDone && len(rec.ResponseJSON) > 0 {
						w.Header().Set("Content-Type", "application/json; charset=utf-8")
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write(rec.ResponseJSON)
						return
					}
					w.WriteHeader(http.StatusConflict)
					_, _ = io.WriteString(w, "idempotency key already used")
					return
				}
			}
		}

		// Decode the already-validated payload so we can pass it to the DB RPC.
		var payload any
		if err := json.Unmarshal(body, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		res, err := store.Push(r.Context(), user.ID, payload)
		if err != nil {
			log.Printf("push store failed user_id=%q err=%q", user.ID, err.Error())
			if idemKey != "" {
				_ = idem.Delete(r.Context(), user.ID, idemScope, idemKey)
			}
			switch err {
			case repo.ErrDBNotConfigured:
				writeHTTPError(w, http.StatusServiceUnavailable, "db not configured", err)
			default:
				writeHTTPError(w, http.StatusInternalServerError, "push failed", err)
			}
			return
		}

		if idemKey != "" {
			_ = idem.SetDone(r.Context(), user.ID, idemScope, idemKey, res)
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(res)
	})
}
