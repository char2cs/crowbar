package v0

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
)

// withWatcherLifecycle attaches the Files∪Git watcher subscription triggers to a
// StreamDef, scoping the refcount by a chat client's bound workspace or else
// scopeWsID, and delegating to the app-layer realtime service.
func withWatcherLifecycle[T any](
	def ws.StreamDef[T],
	appContainer *app.Container,
) ws.StreamDef[T] {
	def.ScopeKey = scopeWsID
	def.OnSubscribe = appContainer.Realtime.AcquireWatcher
	def.OnUnsubscribe = appContainer.Realtime.ReleaseWatcher
	return def
}

// filesDef serves the home mount (wsId, injected by RequireHomeWorkspace) and
// the chat mount (a bound chatId) from one broadcaster; each client activates
// only the filter its route binds, so neither is Required. No snapshot: a
// file change is news, not state.
func filesDef(
	appContainer *app.Container,
) ws.StreamDef[domain.FileChangeEvent] {
	return ws.StreamDef[domain.FileChangeEvent]{
		Namespace:     func(e domain.FileChangeEvent) string { return e.WsID },
		Serialize:     func(e domain.FileChangeEvent) ([]byte, error) { return json.Marshal(e) },
		FlatNamespace: true,
		Filters: []ws.FilterDef[domain.FileChangeEvent]{
			{Param: "wsId", Extract: func(e domain.FileChangeEvent) string { return e.WsID }, Match: ws.ExactMatch},
			{
				Param:   "chatId",
				Extract: func(e domain.FileChangeEvent) string { return e.WsID },
				Match:   ws.ExactMatch,
				Resolve: chatWorkspaces(appContainer),
			},
		},
	}
}

// documentsDef serves GET /v0/documents/ws: each client holds the one host
// path its query names and hears that path's changes alone. No snapshot: the
// client reads the document once its socket is open.
func documentsDef() ws.StreamDef[domain.FileChangeEvent] {
	path := func(e domain.FileChangeEvent) string { return e.Path }
	return ws.StreamDef[domain.FileChangeEvent]{
		Namespace:     path,
		Serialize:     func(e domain.FileChangeEvent) ([]byte, error) { return json.Marshal(e) },
		FlatNamespace: true,
		Filters: []ws.FilterDef[domain.FileChangeEvent]{
			{Param: "path", Extract: path, Match: ws.ExactMatch, Required: true},
		},
		ScopeKey: func(c *gin.Context) string { return c.Query("path") },
	}
}

// withHostWatchLifecycle holds each client's path through the realtime
// service: its watch is armed before the upgrade answers, and an arm that
// fails refuses the upgrade.
func withHostWatchLifecycle(
	def ws.StreamDef[domain.FileChangeEvent],
	appContainer *app.Container,
) ws.StreamDef[domain.FileChangeEvent] {
	def.Admit = hostWatchAdmit(appContainer.Realtime.AcquireHostWatch)
	def.OnUnsubscribe = appContainer.Realtime.ReleaseHostWatch
	return def
}

// hostWatchAdmit refuses an arm the OS has no watches left for with 507, which
// the client tells the user apart from a daemon that cannot watch the file.
func hostWatchAdmit(
	acquire func(path string) error,
) func(path string) error {
	return func(path string) error {
		err := acquire(path)
		if errors.Is(err, enginefs.ErrWatchLimit) {
			return &ws.AdmitRefusal{Status: http.StatusInsufficientStorage, Err: err}
		}
		return err
	}
}
