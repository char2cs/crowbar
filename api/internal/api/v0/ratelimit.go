package v0

import (
	"maps"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// maxLimitedSources bounds the per-source table. Evicting past it only hands a
// source a fresh window; the outstanding code's own failure cap still bounds a
// brute force spread over many addresses.
const maxLimitedSources = 4096

// ipv6HostBits keys an IPv6 peer by its /64, the block a single host controls.
const ipv6HostBits = 64

type sourceWindow struct {
	count int
	ends  time.Time
}

// sourceLimiter rejects a source's requests past max within one fixed window,
// reset (not decayed) once the window elapses. Keying per source means one
// caller's wrong guesses never lock another caller out.
type sourceLimiter struct {
	max    int
	window time.Duration
	now    func() time.Time

	mu      sync.Mutex
	sources map[string]*sourceWindow
}

func newSourceLimiter(
	max int,
	window time.Duration,
	now func() time.Time,
) *sourceLimiter {
	return &sourceLimiter{max: max, window: window, now: now, sources: map[string]*sourceWindow{}}
}

// handle keys on the TCP peer alone: a forwarded-for header is the caller's to
// set, so trusting one would let any caller pick a fresh budget per request.
func (l *sourceLimiter) handle(
	c *gin.Context,
) {
	if !l.allow(sourceOf(c.Request.RemoteAddr)) {
		libs.WriteErr(c, http.StatusTooManyRequests, "too many requests, try again shortly")
		c.Abort()
		return
	}
	c.Next()
}

func (l *sourceLimiter) allow(
	source string,
) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	w, ok := l.sources[source]
	if !ok || now.After(w.ends) {
		l.makeRoom(now)
		w = &sourceWindow{ends: now.Add(l.window)}
		l.sources[source] = w
	}
	w.count++
	return w.count <= l.max
}

func (l *sourceLimiter) makeRoom(
	now time.Time,
) {
	if len(l.sources) < maxLimitedSources {
		return
	}
	maps.DeleteFunc(l.sources, func(_ string, w *sourceWindow) bool { return now.After(w.ends) })
	if len(l.sources) < maxLimitedSources {
		return
	}
	l.evictOldest()
}

func (l *sourceLimiter) evictOldest() {
	var (
		oldest     string
		oldestEnds time.Time
	)
	for source, w := range l.sources {
		if oldestEnds.IsZero() || w.ends.Before(oldestEnds) {
			oldest, oldestEnds = source, w.ends
		}
	}
	delete(l.sources, oldest)
}

func sourceOf(
	remoteAddr string,
) string {
	peer, err := netip.ParseAddrPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	addr := peer.Addr().Unmap()
	if addr.Is4() {
		return addr.String()
	}
	return netip.PrefixFrom(addr.WithZone(""), ipv6HostBits).Masked().String()
}
