package app

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/app/stream/resources"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	domainstream "github.com/char2cs/crowbar/api/internal/domain/stream"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// lateStream lets what is built before the stream publish to it: the stream's
// sources read the usecases, and some usecases publish.
type lateStream struct{ hub *stream.Hub }

func (l *lateStream) operation(op domain.Operation) {
	l.hub.Entity(domainstream.Projects, domainstream.KindOperation, op.ID, op)
}

// reviewThread is how an agent's review comment reaches an open review pane,
// exactly as one written over REST does.
func (l *lateStream) reviewThread() agentusecase.ToolThreadBroadcast {
	return func(thread domain.ReviewThread, projectID, repoID string) {
		scope := domainstream.Scope{Kind: domainstream.KindWs, Key: thread.WsID}
		l.hub.Entity(scope, domainstream.KindReviewThread, thread.ID,
			stream.ReviewThread{Thread: thread, ProjectID: projectID, RepoID: repoID})
	}
}

// newStream builds the stream over every scope's source and the resources a
// held scope runs.
func newStream(
	ctx context.Context,
	late *lateStream,
	encode stream.Encoder,
	ops *operation.Service,
	repos *repositories.Container,
	ucs *usecases.Container,
	engines *engine.Container,
) *resources.Set {
	set := resources.NewSet(ctx, signals{late}, resources.Deps{
		Workspace: repos.Workspace,
		Git:       engines.Git,
		FS:        engines.FS,
		LSP:       engines.LSP,
		Forge:     ucs.ProviderSync,
		Now:       time.Now,
	})
	late.hub = stream.New(uuid.NewString(), map[domainstream.ScopeKind]stream.Source{
		domainstream.KindProjects: stream.NewProjectsSource(ops, encode),
		domainstream.KindWs:       stream.NewWsSource(repos.Workspace, repos.ReviewThread, encode),
		domainstream.KindGit:      stream.Unordered(stream.NewGitSource(repos.Workspace, ucs.Git)),
		domainstream.KindFiles:    stream.Quiet{},
		domainstream.KindLsp:      stream.Unordered(stream.NewLspSource(engines.LSP)),
		domainstream.KindDoc:      stream.NewDocSource(admitHostDocument(ucs)),
	}, set.ByKind(), encode)
	engines.LSP.OnDiagnostics(func(ev lspdomain.DiagnosticsEvent) {
		late.hub.Signal(domainstream.Scope{Kind: domainstream.KindLsp, Key: ev.WsID}, domainstream.SignalDiagnostics, ev)
	})
	return set
}

// signals is the late stream as the resources see it.
type signals struct{ late *lateStream }

func (s signals) Signal(scope domainstream.Scope, kind string, data any) {
	s.late.hub.Signal(scope, kind, data)
}

// admitHostDocument lets a client watch only a path it could read: a canonical
// host path, over the local socket the stream request arrived on.
func admitHostDocument(ucs *usecases.Container) func(ctx context.Context, path string) error {
	return func(ctx context.Context, path string) error {
		if _, err := ucs.File.HostDocument(ctx, path, domain.TransportOf(ctx)); err != nil {
			return fmt.Errorf("app: doc %s: %w", path, err)
		}
		return nil
	}
}
