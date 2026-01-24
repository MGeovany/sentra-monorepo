package httpapi

import (
	"net"
	"net/http"
	"os"
	"strings"
)

func requireLoopback(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}

	// Default secure in dev: only accept loopback connections.
	v := strings.TrimSpace(os.Getenv("SENTRA_LOOPBACK_ONLY"))
	if v == "" {
		v = "1"
	}
	if v == "0" || strings.EqualFold(v, "false") {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
		if err != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
