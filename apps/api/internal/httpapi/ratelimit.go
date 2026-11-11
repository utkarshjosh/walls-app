package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// limiter allows at most max events per key in each window of time. It lives
// in memory only: a restart forgets it, which is fine for slowing down abuse.
type limiter struct {
	window time.Duration
	max    int

	mu   sync.Mutex
	seen map[string]*span
}

type span struct {
	start time.Time
	n     int
}

// maxLimiterKeys bounds memory if someone sends a flood of made-up addresses.
const maxLimiterKeys = 20_000

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{window: window, max: max, seen: map[string]*span{}}
}

// current returns the key's live span, or nil if it has none or it ran out.
// The caller holds the lock.
func (l *limiter) current(key string, now time.Time) *span {
	sp := l.seen[key]
	if sp != nil && now.Sub(sp.start) >= l.window {
		delete(l.seen, key)
		return nil
	}
	return sp
}

// full reports whether key has used up its allowance, without using any.
func (l *limiter) full(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	sp := l.current(key, now)
	return sp != nil && sp.n >= l.max
}

// add counts one event against key.
func (l *limiter) add(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	sp := l.current(key, now)
	if sp == nil {
		if len(l.seen) >= maxLimiterKeys {
			for k, old := range l.seen {
				if now.Sub(old.start) >= l.window {
					delete(l.seen, k)
				}
			}
			if len(l.seen) >= maxLimiterKeys {
				clear(l.seen) // a flood of keys: start over rather than grow
			}
		}
		sp = &span{start: now}
		l.seen[key] = sp
	}
	sp.n++
}

// allow counts one event against key and reports whether it was within the allowance.
func (l *limiter) allow(key string, now time.Time) bool {
	if l.full(key, now) {
		return false
	}
	l.add(key, now)
	return true
}

// clientKey names the connection a request came from, for rate limits only.
// It is never stored or logged.
//
// The server sits behind Cloudflare and Caddy, so the socket's own address is
// Caddy's. Cloudflare puts the visitor's address in CF-Connecting-IP and Caddy
// passes it through. Someone who reaches the server without Cloudflare can
// make this header up; the limits that matter therefore also have a total
// across all keys.
func clientKey(r *http.Request) string {
	key := strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))
	if key == "" {
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			key = host
		} else {
			key = r.RemoteAddr
		}
	}
	if len(key) > 64 {
		key = key[:64]
	}
	return key
}
