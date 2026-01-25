package httpapi

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mgeovany/sentra/server/internal/auth"
	"github.com/mgeovany/sentra/server/internal/repo"
	"github.com/mgeovany/sentra/server/internal/validate"
)

type registerMachineRequest struct {
	MachineID     string `json:"machine_id"`
	MachineName   string `json:"machine_name"`
	DevicePubKey  string `json:"device_pub_key"`
	DeviceKeyType string `json:"device_key_type"`
}

func registerMachineHandler(store repo.MachineStore) http.Handler {
	if store == nil {
		store = repo.DisabledMachineStore{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB

		user, ok := auth.UserFromContext(r.Context())
		if !ok || strings.TrimSpace(user.ID) == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var req registerMachineRequest
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		req.MachineID = strings.TrimSpace(req.MachineID)
		req.MachineName = strings.TrimSpace(req.MachineName)
		req.DevicePubKey = strings.TrimSpace(req.DevicePubKey)
		req.DeviceKeyType = strings.TrimSpace(req.DeviceKeyType)
		if validate.MachineID(req.MachineID) != nil || validate.MachineName(req.MachineName) != nil || validate.DevicePubKeyB64(req.DevicePubKey) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.DeviceKeyType == "" {
			req.DeviceKeyType = "ed25519"
		}
		if req.DeviceKeyType != "ed25519" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Device binding bootstrap: require a valid device signature using the provided public key.
		// This prevents registering an arbitrary key without proof of possession.
		machineIDHdr := strings.TrimSpace(r.Header.Get("X-Sentra-Machine-ID"))
		ts := strings.TrimSpace(r.Header.Get("X-Sentra-Timestamp"))
		sig := strings.TrimSpace(r.Header.Get("X-Sentra-Signature"))
		nonce := strings.TrimSpace(r.Header.Get("X-Sentra-Nonce"))
		if machineIDHdr == "" || ts == "" || sig == "" || machineIDHdr != req.MachineID {
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
		if err := auth.VerifyDeviceSignature(req.DevicePubKey, req.MachineID, ts, nonce, r.Method, r.URL.Path, body, sig); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "unauthorized")
			return
		}
		key := strings.TrimSpace(user.ID) + "\n" + req.MachineID + "\n" + nonce
		if recentNonces.seenOrMark(key, 10*time.Minute) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "unauthorized")
			return
		}

		// If already registered, do not allow changing the stored device key.
		existing, ok, err := store.DevicePubKey(r.Context(), user.ID, req.MachineID)
		if err != nil {
			log.Printf("machines/register device key lookup failed user_id=%q machine_id=%q err=%v", user.ID, req.MachineID, err)
			switch err {
			case repo.ErrDBNotConfigured:
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, "db not configured")
			default:
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, "device key lookup failed")
			}
			return
		}
		if ok && strings.TrimSpace(existing) != req.DevicePubKey {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, "device key mismatch")
			return
		}

		err = store.Register(r.Context(), user.ID, req.MachineID, req.MachineName, req.DevicePubKey)
		if err != nil {
			// Server-side logging for debugging/observability.
			log.Printf("machines/register failed user_id=%q machine_id=%q machine_name=%q err=%v", user.ID, req.MachineID, req.MachineName, err)
			switch err {
			case repo.ErrDBNotConfigured:
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, "db not configured")
			case repo.ErrTooManyMachines:
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, "too many machines")
			default:
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, "machine register failed")
			}
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})
}
