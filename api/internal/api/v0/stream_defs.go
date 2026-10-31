package v0

import (
	"encoding/json"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app"
)

// projectsDef serves the Projects topic. Its hierarchical namespace is the bare
// project id (spec §5). The snapshot returns every project as a wire DTO from
// the GORM store; the per-client prefix predicate filters it (spec §9).
func projectsDef(
	appContainer *app.Container,
) ws.StreamDef[dto.ProjectDTO] {
	return ws.StreamDef[dto.ProjectDTO]{
		Namespace: func(d dto.ProjectDTO) string { return d.ID },
		Serialize: func(d dto.ProjectDTO) ([]byte, error) { return json.Marshal(d) },
		Snapshot:  projectSnapshot(appContainer),
	}
}

// reposDef serves the Repos topic. Its hierarchical namespace is projectID/ID,
// so a project-scoped subscription ("p") receives every child repo (spec §5).
// The snapshot is project-scoped from the client's subscription prefix and reads
// the repos under that project from the GORM store (spec §9).
func reposDef(
	appContainer *app.Container,
) ws.StreamDef[dto.RepoDTO] {
	return ws.StreamDef[dto.RepoDTO]{
		Namespace: func(d dto.RepoDTO) string { return d.ProjectID + "/" + d.ID },
		Serialize: func(d dto.RepoDTO) ([]byte, error) { return json.Marshal(d) },
		Snapshot:  repoSnapshot(appContainer),
	}
}

// agentChatDef serves the agent-chat lifecycle event stream (GET
// .../repos/:repoId/chats/ws, and still GET .../home/chats/ws). It carries no
// snapshot: unlike the full-state resource streams above (projects, repos,
// ...) a freshly-connected client simply waits for the next
// lifecycle event — there is no "current state" to replay.
//
// It has FOUR scoping filters and stays FlatNamespace, which is the shape the
// rows themselves force. The wsId Filter is what narrows the HOME mount, whose
// RequireHomeWorkspace injects a :wsId for it to resolve; it goes inactive at
// the repo mount, which binds no :wsId at all — and that inactive filter used
// to be the whole of this stream's scoping, so a repo-scoped client received
// every OTHER repo's chat events too. The repoId Filter closes that: the repo
// mount binds :repoId, and every chat frame carries the repo its row actually
// runs in (see agent_chat_scope.go).
//
// The projectId Filter closes what repoId structurally CANNOT. Both live mounts
// nest under /projects/:projectId, so both resolve it; the flat
// /v0/chats/:chatId/ws mount binds neither it nor :repoId and is scoped by its
// own chatId Filter instead, so neither is Required.
//
// The stream is NOT given the hierarchical projectId/repoId namespace
// reposDef uses, because half the rows on this feed cannot
// fill one. A FOLDER carries no workspace and no repo id of its own, neither
// does a bubble at the panel root, and neither does ANY row in a project home —
// that workspace owns no repo; under a hierarchical namespace those frames
// would resolve "//" and be dropped from every repo-scoped subscriber, silently
// killing the live folder feed. matchScopeOrUnscoped is what lets both kinds
// coexist on one stream: a frame that KNOWS a scope is held to it, and one that
// cannot know it reaches everyone.
//
// That escape hatch is why the two filters are needed rather than one. It is
// keyed per FIELD, so a repo-less frame skipped the repoId filter entirely —
// and a project home's chats are all repo-less, so a home chat's streamed text
// in one project was delivered to repo-scoped sockets in EVERY other project in
// the process. Answering the project as well bounds them: repo-less still means
// "every repo", but now only every repo OF THIS PROJECT, which is the narrowest
// scope that keeps the folder rows and root bubbles this hatch exists for. A
// frame that resolves neither field still reaches everyone — the same disclosed
// limitation ChatTreeUsecase.ListInRepo already carries for folders, unchanged,
// rather than a new silent drop.
func agentChatDef() ws.StreamDef[dto.AgentChatEvent] {
	return ws.StreamDef[dto.AgentChatEvent]{
		Namespace:     func(e dto.AgentChatEvent) string { return e.WorkspaceID },
		Serialize:     func(e dto.AgentChatEvent) ([]byte, error) { return json.Marshal(e) },
		Snapshot:      func(string) []dto.AgentChatEvent { return nil },
		FlatNamespace: true,
		Filters: []ws.FilterDef[dto.AgentChatEvent]{
			{Param: "wsId", Extract: func(e dto.AgentChatEvent) string { return e.WorkspaceID }, Match: ws.ExactMatch},
			{Param: "projectId", Extract: func(e dto.AgentChatEvent) string { return e.ProjectID }, Match: matchScopeOrUnscoped},
			{Param: "repoId", Extract: func(e dto.AgentChatEvent) string { return e.RepoID }, Match: matchScopeOrUnscoped},
			{Param: "chatId", Extract: func(e dto.AgentChatEvent) string { return e.ChatID }, Match: ws.ExactMatch},
		},
		// message_delta is the one kind on this feed that is already "the
		// full state so far" by construction (see
		// hub.BroadcastAgentChatMessageDelta's own doc comment) — a client
		// that misses several is exactly as correct as one that saw every
		// one, as long as it eventually sees the latest. A fast-streaming
		// provider can emit these far more often than this feed's other,
		// genuinely stateful kinds (turn_started, folder events, ...), and
		// those must keep the ordinary bounded-queue-then-disconnect
		// contract untouched — CoalesceKey is what lets the two coexist on
		// one broadcaster. Keyed by (chat, stream kind, message id) so two chats,
		// two co-open items in one chat's turn, or the answer and the thinking
		// that runs alongside it never coalesce into each other. The kind is in
		// the key structurally rather than because the two id spaces happen to
		// differ today: collapsing a thought onto an answer would replace the
		// reply with the reasoning that preceded it.
		CoalesceKey: func(e dto.AgentChatEvent) (string, bool) {
			if e.Kind != dto.AgentChatKindMessageDelta || e.Message == nil {
				return "", false
			}
			return e.ChatID + "\x00" + e.Message.Kind + "\x00" + e.Message.ID, true
		},
	}
}

// matchScopeOrUnscoped holds a frame that KNOWS one of its scoping ids to
// exactly that id, and lets one that does not reach every subscriber. It serves
// both of agentChatDef's scoping Filters, projectId and repoId.
//
// The second half is not laxness, it is the honest answer for the rows that
// have no such id to be held to: a folder row carries no repo, neither does a
// bubble whose ancestry owns no workspace, and neither does any row in a
// project home. Refusing those would drop the live folder feed the Chats panel
// repaints from. ws.ExactMatch would do exactly that, which is why this is its
// own function and not that one.
//
// It is also why ONE of these filters is not enough. The hatch opens per FIELD,
// so a repo-less frame was held to nothing at all — it took a second field the
// same row CAN answer to bound it. See agentChatDef.
func matchScopeOrUnscoped(
	param string,
	value string,
) bool {
	return value == "" || param == value
}
