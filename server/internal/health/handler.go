package health

import (
	"io"
	"net/http"
)

func Register(mux *http.ServeMux) {
	ok := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}

	mux.HandleFunc("/health", ok)
	mux.HandleFunc("/healthz", ok)
	mux.HandleFunc("/livez", ok)
	mux.HandleFunc("/readyz", ok)
}

func Handler() http.Handler {
	mux := http.NewServeMux()
	Register(mux)
	return mux
}
