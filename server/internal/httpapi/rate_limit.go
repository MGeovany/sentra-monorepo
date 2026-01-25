package httpapi

import (
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mgeovany/sentra/server/internal/auth"
)

type tokenBucket struct {
	tokens float64
	last   time.Time
}

type tokenBuckets struct {
	mu sync.Mutex
	m  map[string]tokenBucket
}

func (b *tokenBuckets) take(key string, now time.Time, ratePerSec, burst float64) (ok bool, retryAfter time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.m == nil {
		b.m = make(map[string]tokenBucket)
	}

	// Lazy cleanup to keep memory bounded.
	for k, v := range b.m {
		if now.Sub(v.last) > 30*time.Minute {
			delete(b.m, k)
		}
	}

	v := b.m[key]
	if v.last.IsZero() {
		v.tokens = burst
		v.last = now
	}

	// Refill.
	if now.After(v.last) {
		v.tokens = math.Min(burst, v.tokens+now.Sub(v.last).Seconds()*ratePerSec)
		v.last = now
	}

	if v.tokens >= 1 {
		v.tokens -= 1
		b.m[key] = v
		return true, 0
	}

	missing := 1 - v.tokens
	seconds := missing / ratePerSec
	retryAfter = time.Duration(math.Ceil(seconds)) * time.Second
	// No token consumed, but update last so idle cleanup works.
	b.m[key] = v
	return false, retryAfter
}

var pushBuckets tokenBuckets

var machineRegisterBuckets tokenBuckets

func requireMachineRegisterRateLimit(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}

	// Registration should be rare; keep tight to reduce abuse.
	rpm := int64(30)
	burst := float64(10)
	ratePerSec := float64(rpm) / 60.0

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.UserFromContext(r.Context())
		if !ok || strings.TrimSpace(user.ID) == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		allowed, retryAfter := machineRegisterBuckets.take(user.ID, time.Now().UTC(), ratePerSec, burst)
		if !allowed {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("rate limit exceeded; retry later"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func requirePushRateLimit(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}

	// Default is tuned for local (loopback-only) pushes.
	// A single "sentra push" can fan out into many per-project requests.
	rpm := int64(300)
	if v := strings.TrimSpace(os.Getenv("SENTRA_PUSH_RPM")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			rpm = n
		}
	}

	burst := float64(60)
	if v := strings.TrimSpace(os.Getenv("SENTRA_PUSH_BURST")); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			burst = n
		}
	}

	ratePerSec := float64(rpm) / 60.0

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.UserFromContext(r.Context())
		if !ok || strings.TrimSpace(user.ID) == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		allowed, retryAfter := pushBuckets.take(user.ID, time.Now().UTC(), ratePerSec, burst)
		if !allowed {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("rate limit exceeded; retry later"))
			return
		}

		next.ServeHTTP(w, r)
	})
}
