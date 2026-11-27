package stream

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type ScopeKind string

const (
	KindProjects ScopeKind = "projects"
	KindChat     ScopeKind = "chat"
	KindWs       ScopeKind = "ws"
	KindGit      ScopeKind = "git"
	KindFiles    ScopeKind = "files"
	KindLsp      ScopeKind = "lsp"
	KindDoc      ScopeKind = "doc"
	KindForge    ScopeKind = "forge"
)

var keyed = map[ScopeKind]bool{
	KindChat: true, KindWs: true, KindGit: true, KindFiles: true, KindLsp: true, KindDoc: true, KindForge: true,
}

type Scope struct {
	Kind ScopeKind
	Key  string
}

var Projects = Scope{Kind: KindProjects}

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
