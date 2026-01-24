package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mgeovany/sentra/server/internal/auth"
	"github.com/mgeovany/sentra/server/internal/config"
	"github.com/mgeovany/sentra/server/internal/httpapi"
	"github.com/mgeovany/sentra/server/internal/repo"
	"github.com/mgeovany/sentra/server/internal/supabase"
)

func main() {
	config.LoadDotEnv()
	cfg := config.FromEnv()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var verifier auth.Verifier = auth.DisabledVerifier{}
	if cfg.SupabaseURL != "" {
		verifier = auth.NewJWKSVerifier(cfg.SupabaseURL)
		log.Printf("auth jwks configured")
	}

	middleware := auth.NewMiddleware(verifier)

	var machines repo.MachineStore = repo.DisabledMachineStore{}
	var push repo.PushStore = repo.DisabledPushStore{}
	if cfg.SupabaseURL != "" && cfg.SupabaseServiceRoleKey != "" {
		client, err := supabase.New(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey)
		if err != nil {
			log.Printf("supabase db disabled (check SUPABASE_URL / SUPABASE_SERVICE_ROLE_KEY)")
		} else {
			machines = repo.NewSupabaseMachineStore(client, cfg.SupabaseMachinesTable)
			push = repo.NewSupabasePushStore(client, "")
			log.Printf("supabase db configured")
		}
	}

	h := httpapi.New(httpapi.Deps{Auth: middleware, Machines: machines, Push: push})

	srv := &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, cfg.Port),
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
