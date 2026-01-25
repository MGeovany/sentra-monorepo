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

func exportHandler(store repo.ExportStore) http.Handler {
	if store == nil {
		store = repo.DisabledExportStore{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		user, ok := auth.UserFromContext(r.Context())
		if !ok || strings.TrimSpace(user.ID) == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		root := strings.TrimSpace(r.URL.Query().Get("root"))
		if root == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "missing root")
			return
		}
		at := strings.TrimSpace(r.URL.Query().Get("at"))

		files, err := store.Export(r.Context(), user.ID, root, at)
		if err != nil {
			log.Printf("export failed user_id=%s root=%s err=%v", user.ID, root, err)
			switch err {
			case repo.ErrDBNotConfigured:
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, "db not configured")
			default:
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, "export failed")
			}
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(files)
	})
}
