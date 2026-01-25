package httpapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mgeovany/sentra/server/internal/auth"
	"github.com/mgeovany/sentra/server/internal/repo"
	"github.com/mgeovany/sentra/server/internal/validate"
)

type ctxKeySignedBody struct{}

const maxPushBodyBytes = 12 << 20 // 12 MiB

type nonceCache struct {
	mu sync.Mutex
	m  map[string]time.Time
}

func (c *nonceCache) seenOrMark(key string, ttl time.Duration) bool {
	now := time.Now()
	exp := now.Add(ttl)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = make(map[string]time.Time)
	}
	// Lazy cleanup.
	for k, v := range c.m {
		if now.After(v) {
			delete(c.m, k)
		}
	}
	if t, ok := c.m[key]; ok && now.Before(t) {
		return true
	}
	c.m[key] = exp
	return false
}

var recentNonces nonceCache

func requireDeviceSignature(store repo.MachineStore, next http.Handler) http.Handler {
	if store == nil {
		store = repo.DisabledMachineStore{}
	}
	if next == nil {
		next = http.NotFoundHandler()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxPushBodyBytes)

		user, ok := auth.UserFromContext(r.Context())
		if !ok || strings.TrimSpace(user.ID) == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "unauthorized")
			return
		}

		machineID := strings.TrimSpace(r.Header.Get("X-Sentra-Machine-ID"))
		ts := strings.TrimSpace(r.Header.Get("X-Sentra-Timestamp"))
		sig := strings.TrimSpace(r.Header.Get("X-Sentra-Signature"))
		nonce := strings.TrimSpace(r.Header.Get("X-Sentra-Nonce"))
		if machineID == "" || ts == "" || sig == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "unauthorized")
			return
		}
		if validate.MachineID(machineID) != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "unauthorized")
			return
		}
		if nonce == "" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusUpgradeRequired)
			_, _ = io.WriteString(w, "Please update Sentra CLI (requires nonce-signed requests).")
			return
		}

		pub, pubOK, err := store.DevicePubKey(r.Context(), user.ID, machineID)
		if err != nil {
			if err == repo.ErrDBNotConfigured {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, "db not configured")
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "unauthorized")
			return
		}
		if !pubOK || strings.TrimSpace(pub) == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "unauthorized")
			return
		}

		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))

		if err := auth.VerifyDeviceSignature(pub, machineID, ts, nonce, r.Method, r.URL.Path, body, sig); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "unauthorized")
			return
		}

		// Anti-replay: nonce must be unique for a short TTL.
		key := strings.TrimSpace(user.ID) + "\n" + machineID + "\n" + nonce
		if recentNonces.seenOrMark(key, 10*time.Minute) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "unauthorized")
			return
		}

		r = r.WithContext(context.WithValue(r.Context(), ctxKeySignedBody{}, body))

		next.ServeHTTP(w, r)
	})
}
