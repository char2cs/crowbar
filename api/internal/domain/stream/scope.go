// Package stream declares the one daemon-to-client stream's vocabulary: the
// scopes a client holds and the frames it is sent.
package stream

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// ScopeKind is what a scope is about; a kind other than projects names one
// entity by its key.
type ScopeKind string

// The scope kinds a client may hold.
const (
	KindProjects ScopeKind = "projects"
	KindChat     ScopeKind = "chat"
	KindWs       ScopeKind = "ws"
	KindGit      ScopeKind = "git"
	KindFiles    ScopeKind = "files"
	KindLsp      ScopeKind = "lsp"
)

var keyed = map[ScopeKind]bool{KindChat: true, KindWs: true, KindGit: true, KindFiles: true, KindLsp: true}

// Scope is one thing a client holds: "projects", or "<kind>:<key>".
type Scope struct {
	Kind ScopeKind
	Key  string
}

// Projects is the scope of every project's entities.
var Projects = Scope{Kind: KindProjects}

// ParseScope reads a scope as a client spells it.
func ParseScope(s string) (Scope, error) {
	if ScopeKind(s) == KindProjects {
		return Projects, nil
	}
	kind, key, _ := strings.Cut(s, ":")
	if !keyed[ScopeKind(kind)] {
		return Scope{}, problem.New(problem.InvalidArgument, fmt.Sprintf("stream: unknown scope kind %q", kind))
	}
	if key == "" {
		return Scope{}, problem.New(problem.InvalidArgument, fmt.Sprintf("stream: scope %q names no key", s))
	}
	return Scope{Kind: ScopeKind(kind), Key: key}, nil
}

func (s Scope) String() string {
	if s.Key == "" {
		return string(s.Kind)
	}
	return string(s.Kind) + ":" + s.Key
}
