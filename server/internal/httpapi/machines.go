package httpapi

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/mgeovany/sentra/server/internal/auth"
	"github.com/mgeovany/sentra/server/internal/repo"
)

type registerMachineRequest struct {
	MachineID   string `json:"machine_id"`
	MachineName string `json:"machine_name"`
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

		user, ok := auth.UserFromContext(r.Context())
		if !ok || strings.TrimSpace(user.ID) == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var req registerMachineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		req.MachineID = strings.TrimSpace(req.MachineID)
		req.MachineName = strings.TrimSpace(req.MachineName)
		if req.MachineID == "" || req.MachineName == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err := store.Register(r.Context(), user.ID, req.MachineID, req.MachineName)
		if err != nil {
			// Server-side logging for debugging/observability.
			log.Printf("machines/register failed user_id=%s machine_id=%s machine_name=%s err=%v", user.ID, req.MachineID, req.MachineName, err)
			switch err {
			case repo.ErrDBNotConfigured:
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, "db not configured")
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
