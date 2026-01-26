package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/mgeovany/sentra/server/internal/auth"
	"github.com/mgeovany/sentra/server/internal/health"
	"github.com/mgeovany/sentra/server/internal/repo"
)

type Deps struct {
	Auth     auth.Middleware
	Machines repo.MachineStore
	Idem     repo.IdempotencyStore
	Projects repo.ProjectStore
	Commits  repo.CommitStore
	Files    repo.FileStore
	Export   repo.ExportStore
	Push     repo.PushStore
}

func New(deps Deps) http.Handler {
	mux := http.NewServeMux()

	health.Register(mux)

	mux.Handle("/users/me", deps.Auth.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := auth.UserFromContext(r.Context())
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(user)
	})))

	mux.Handle("/projects", requireLoopback(deps.Auth.Require(projectsHandler(deps.Projects))))
	mux.Handle("/commits", requireLoopback(deps.Auth.Require(commitsHandler(deps.Commits))))
	mux.Handle("/files", requireLoopback(deps.Auth.Require(filesHandler(deps.Files))))
	mux.Handle("/export", requireLoopback(deps.Auth.Require(exportHandler(deps.Export))))
	mux.Handle("/machines/register", requireLoopback(deps.Auth.Require(requireMachineRegisterRateLimit(registerMachineHandler(deps.Machines)))))
	mux.Handle("/push", requireLoopback(deps.Auth.Require(requirePushRateLimit(requireDeviceSignature(deps.Machines, pushHandler(deps.Push, deps.Idem))))))

	return mux
}
