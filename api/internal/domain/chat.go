package domain

import "time"

// The VIEWS a chat can be on — design spec 2.5. Persistence-side names for
// the same two the descriptor schema declares (engine/agents.SurfaceChat /
// .SurfaceTerminal); TestSurfaceNames_MatchTheDescriptorVocabulary
// (engine/agents) pins them equal, since the string crosses both layers and
// the wire.
//
// "" is a THIRD legitimate value everywhere this appears: the provider's own
// default landing, which is Crowbar's chat.
const (
	SurfaceChat     = "chat"
	SurfaceTerminal = "terminal"
)

// KnownSurface reports whether s names a surface a chat can actually be on.
// "" (the provider's default landing) is one of them — absence of a choice,
// not an invalid choice.
func KnownSurface(s string) bool {
	return s == "" || s == SurfaceChat || s == SurfaceTerminal
}

// Chat is the Crowbar-owned agentic conversation thread. Mutated only
// through asynx commands. Conversation content lives in the ledger, not here —
// this aggregate holds identity, title, live turn state, and a ledger cursor.
//
// It knows nothing about processes: the runner points at the chat, never the
// reverse, so "is this chat live?" asks the supervisor's live table, and the
// conversations it hosted are chat-ledger rows.
type Chat struct {
	ID          string   `json:"id"`
	Type        ChatType `json:"type"`
	WorkspaceID string   `json:"workspaceId"`
	// OwnsWorkspace records that this row is the one that OWNS WorkspaceID —
	// minted for it, forked it, or was promoted into it — as against a thread
	// that merely runs inside the same worktree. Rows from before this field
	// carry false and fall back to ResolveOwningChat's heuristic.
	OwnsWorkspace bool      `json:"ownsWorkspace,omitempty"`
	Title         string    `json:"title"`
	TitleLocked   bool      `json:"titleLocked"`
	CreatedAt     time.Time `json:"createdAt"`

	// RepoID is a FOLDER's (Type == ChatTypeFolder) own repo scope: "" for a
	// project-home folder, a real repo id otherwise. It exists only because a
	// folder owns no workspace to derive one from the way every other row can
	// (WorkspaceID always resolves to a repo — see
	// usecases/chat/repo_scope.go's "derive, do not store" rule, which this
	// field deliberately does NOT extend to non-folder rows: a chat or branch
	// row must never populate this, WorkspaceID remains its one source of
	// truth). Set once at creation from the caller's own scope (the repo-
	// scoped or project-home create route) and never rewritten by a move — a
	// folder cannot change which repo it belongs to by being dragged, only by
	// which SAME-scoped parent it is filed under (tree/validate.go's
	// checkFolderContainer enforces this — the folder-scoping "golden rule").
	RepoID string `json:"repoId,omitempty"`

	// Model and Effort are the chat's STICKY choice of what to run its provider
	// CLI as: durable config beside the title, not a property of any process. They
	// persist between messages so the picker has a value to show and the next
	// message runs under the same choice as the last.
	//
	// EMPTY means "the provider's own default", and that is a distinct fact from
	// any declared value — never a stand-in for one. Nothing substitutes a default
	// into a spawn, so a chat that has chosen nothing produces byte-identical argv
	// to one created before this field existed.
	//
	// They are what the chat WANTS. What its live CLI is actually running is
	// AgentRunner.LaunchModel/LaunchEffort — Crowbar's record of the spawn — and
	// the gap between the two is exactly what makes the next prompt restart the
	// TUI.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`

	// PermissionLevel is the SAME sticky-choice/gap-drives-restart pattern as
	// Model/Effort above (AgentRunner.LaunchPermissionLevel is its own
	// LaunchModel/LaunchEffort), for Crowbar's own guarded/trusted/full-auto
	// dial. It differs from Model/Effort in one way: empty is NOT "the
	// provider's own default" here — a chat is always seeded with a real
	// level at creation, so empty only ever means "not seeded yet," a
	// transient state no chat a client can see should be in.
	//
	// It is NOT, on its own, "what this chat spawns under" — see
	// PermissionLevelExplicit below. ChatSelection is the one place that
	// reads this field and decides what a spawn actually gets.
	PermissionLevel string `json:"permissionLevel,omitempty"`

	// PermissionLevelExplicit distinguishes the two different facts
	// PermissionLevel used to conflate: false means the chat has only ever
	// INHERITED it from whatever the global default happened to be — seeded
	// at mint for display, but re-resolved against the CURRENT global default
	// on every future spawn (ChatSelection), so a later change to the dial
	// reaches this chat too. True means SetChatPermissionLevel pinned an
	// EXPLICIT per-chat choice, which then wins over the global dial for
	// good, exactly like Model/Effort's own "" vs a real value already does.
	// Zero value (false) is correct for every chat that predates this field.
	PermissionLevelExplicit bool `json:"permissionLevelExplicit,omitempty"`

	// Surface is the VIEW this chat is on RIGHT NOW — design spec 2.5's
	// `surfaces.<name>`, "" for the provider's own default landing. It is the
	// SINGLE source of truth for that, and how the chat got here is not
	// recorded anywhere: birth merely SEEDS it (CreateChat), and
	// SwitchToTerminal/SwitchToNative move it.
	//
	// Durable and sticky like Model/Effort beside it, and for the same
	// reason: every respawn (a restart_tui prompt, a model change, a resume
	// after the daemon restarted) rebuilds the process from scratch, and a
	// chat on the terminal that silently came back on the api transport
	// would lose the very view the user is looking at.
	//
	// It decides two things. At spawn, whether an api connection is opened at
	// all (spawnRunner's surfaceForSpawn): a surface whose channel is hooks is
	// fed by the CLI's own PTY, so a connection beside it would fork a second
	// session and hide that PTY. At ingest, which surface an event's own
	// `surfaces:` list is gated against (Runners.ShowingNativeView).
	Surface string `json:"surface,omitempty"`

	// ProviderID is the VENDOR this chat runs as — the same kind of durable,
	// sticky choice as Model/Effort/Surface beside it, and just as much NOT a
	// claim that any process exists. "Is this chat live?" stays a query against
	// the runner read model; this only answers "as whom does it come back".
	//
	// Seeded at birth from the create's own provider and restated by every
	// later spawn (a switch, a /clear that moves a CLI onto another chat), so
	// it names the last CLI Crowbar actually placed here.
	//
	// It exists because the two RUNNER PROJECTIONS that used to answer this
	// question are both blind to the same chat. A provider that binds by its
	// own connection identity writes no conversation row, and a chat BORN on
	// one was never switched, so it has no provider_switched marker either:
	// both sources are empty and the resolvers answered "" — an absence the
	// frontend read as "this chat never ran" and resolved by starting the
	// FIRST ENABLED provider on it. That silently converted dormant codex
	// chats to claude, transcript and all, on nothing but a sidebar click.
	// Chats minted before this field carry "" and keep falling back to the
	// projections exactly as they always did.
	ProviderID string `json:"providerId,omitempty"`

	// ParentID is the row this chat hangs off in the Chats tree: another chat, a
	// folder, or "" at the panel root. It is the ONLY record of the relationship
	// — no badge, no stored fork point — because the relationship is live rather
	// than a snapshot taken at a moment.
	//
	// A chat parent means a THREAD: this chat reads that chat's turns, and its
	// chat ancestors' in turn, as they stand whenever it asks. A folder parent
	// means organisation only; folders hold no turns, so lineage steps straight
	// through them. That single field therefore answers both "where is this row
	// drawn" and "what does this agent read", and it is why a drag in this panel
	// legitimately rewrites lineage where the sidebar's drag may not.
	ParentID string `json:"parentId,omitempty"`

	// Order is this row's dense index within ParentID's sibling space, which
	// chats SHARE with AgentChatFolder rows: the two kinds interleave at every
	// level, so the panel merges both sets and sorts them on this one field.
	Order int `json:"order"`
}

// ChatSelection is a model/effort pick travelling WITH a prompt — the
// composer's staged choice, committed atomically with the message it was
// picked for rather than by a prior write.
//
// A nil *ChatSelection means "nothing staged, leave Chat.Model/Effort exactly
// as they are". A non-nil one is the WHOLE selection, and its zero value is a
// real pick: "back to the provider's own default", the same distinct fact
// Chat.Model/Effort's own doc records. That is why this is a pointer and not
// two strings — "" cannot be both "unset this" and "I said nothing".
//
// The two halves travel together because they are not independent: which
// effort levels exist is a property of the MODEL, so a partial write could
// store a pair that was never jointly valid.
type ChatSelection struct {
	Model  string
	Effort string
}
