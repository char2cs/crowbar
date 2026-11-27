package descriptorcheck

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
)

var ErrBlocked = errors.New("descriptorcheck: descriptor has errors")

type Gate struct {
	mu      sync.Mutex
	reports map[string]gateEntry
}

type gateEntry struct {
	digest [sha256.Size]byte
	report Report
}

func NewGate() *Gate {
	return &Gate{reports: map[string]gateEntry{}}
}

func (g *Gate) Require(homeDir, id string) error {
	src, ok := protocol.DescriptorSourceFor(homeDir, id)
	if !ok {
		return nil
	}
	rep := g.report(src)
	if fallsBack(src, rep) {
		src, _ = protocol.EmbeddedDescriptorSource(id)
		rep = g.report(src)
	}
	for _, f := range rep.Findings {
		if f.Severity == SeverityError {
			return fmt.Errorf("%w: %s (line %d): %s", ErrBlocked, id, f.Line, f.Message)
		}
	}
	return nil
}

func (g *Gate) report(src protocol.DescriptorSource) Report {
	digest := sha256.Sum256(src.Raw)
	key := src.ID + "\x00" + src.Path
	g.mu.Lock()
	cached, ok := g.reports[key]
	g.mu.Unlock()
	if ok && cached.digest == digest {
		return cached.report
	}
	rep := Validate(src.Raw)
	g.mu.Lock()
	g.reports[key] = gateEntry{digest: digest, report: rep}
	g.mu.Unlock()
	return rep
}
