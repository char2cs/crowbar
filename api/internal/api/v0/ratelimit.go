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

const maxLimitedSources = 4096

const ipv6HostBits = 64

type sourceWindow struct {
	count int
	ends  time.Time
}

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
