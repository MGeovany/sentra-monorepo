package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/mgeovany/sentra/server/internal/auth"
	"github.com/mgeovany/sentra/server/internal/repo"
)

type Deps struct {
	Auth     auth.Middleware
	Machines repo.MachineStore
	Projects repo.ProjectStore
	Commits  repo.CommitStore
	Files    repo.FileStore
	Export   repo.ExportStore
	Push     repo.PushStore
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

	mux.Handle("/projects", requireLoopback(deps.Auth.Require(projectsHandler(deps.Projects))))
	mux.Handle("/commits", requireLoopback(deps.Auth.Require(commitsHandler(deps.Commits))))
	mux.Handle("/files", requireLoopback(deps.Auth.Require(filesHandler(deps.Files))))
	mux.Handle("/export", requireLoopback(deps.Auth.Require(exportHandler(deps.Export))))
	mux.Handle("/machines/register", requireLoopback(deps.Auth.Require(registerMachineHandler(deps.Machines))))
	mux.Handle("/push", requireLoopback(deps.Auth.Require(requirePushRateLimit(requireDeviceSignature(deps.Machines, pushHandler(deps.Push))))))

	return mux
}
