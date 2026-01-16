package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mgeovany/sentra/server/internal/auth"
	"github.com/mgeovany/sentra/server/internal/config"
	"github.com/mgeovany/sentra/server/internal/httpapi"
)

func main() {
	config.LoadDotEnv()
	cfg := config.FromEnv()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var verifier auth.Verifier = auth.DisabledVerifier{}
	if cfg.SupabaseURL != "" {
		jwksURL := cfg.SupabaseURL + "/auth/v1/.well-known/jwks.json"
		verifier = auth.NewJWKSVerifier(jwksURL)
		log.Printf("auth jwks configured")
	}

	middleware := auth.NewMiddleware(verifier)

	h := httpapi.New(httpapi.Deps{Auth: middleware})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
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
