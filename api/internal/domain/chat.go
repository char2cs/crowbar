package domain

import "time"

// The VIEWS a chat can be on, equal to the descriptor schema's surface names (pinned by a test in
// engine/agents) since the string crosses both layers and the wire. "" is also legitimate: the
// provider's own default landing, Crowbar's chat.
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

// Chat is the Crowbar-owned conversation thread, mutated only through asynx commands; content lives
// in the ledger. It knows nothing about processes: "is this chat live?" asks the supervisor.
type Chat struct {
	ID          string   `json:"id"`
	Type        ChatType `json:"type"`
	WorkspaceID string   `json:"workspaceId"`
	// OwnsWorkspace marks the row that owns WorkspaceID (minted, forked or promoted into it), not one
	// merely running in the same worktree. Older rows carry false and use ResolveOwningChat's heuristic.
	OwnsWorkspace bool      `json:"ownsWorkspace,omitempty"`
	Title         string    `json:"title"`
	TitleLocked   bool      `json:"titleLocked"`
	CreatedAt     time.Time `json:"createdAt"`

	// RepoID is a FOLDER's own repo scope ("" for project home): a folder owns no workspace to derive it
	// from. Chat and branch rows never set it; WorkspaceID stays their source of truth. Set once at
	// creation and never rewritten by a move — a folder cannot change repo by being dragged.
	RepoID string `json:"repoId,omitempty"`

	// Model and Effort are the chat's sticky choice of what to run, kept so the next message runs the same.
	// Empty means "the provider's own default" and is never substituted. The live process's actual choice
	// is AgentRunner.LaunchModel/LaunchEffort; a gap between the two restarts the TUI on the next prompt.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`

	// PermissionLevel follows the same sticky-choice/gap-restarts pattern for Crowbar's permission dial,
	// but empty only means "not seeded yet". ChatSelection alone decides what a spawn gets (see
	// PermissionLevelExplicit).
	PermissionLevel string `json:"permissionLevel,omitempty"`

	// PermissionLevelExplicit is false while the level is only inherited (re-resolved against the
	// current global default on every spawn) and true once SetChatPermissionLevel pinned a per-chat
	// choice that wins over the dial. Older chats are correctly false.
	PermissionLevelExplicit bool `json:"permissionLevelExplicit,omitempty"`

	// Surface is the single source of truth for the VIEW this chat is on now ("" = provider default).
	// Sticky because every respawn rebuilds the process and must not lose the user's view. It decides
	// whether a spawn opens an api connection (a hooks surface must not fork a second session) and gating.
	Surface string `json:"surface,omitempty"`

	// ProviderID is the vendor this chat comes back as, seeded at birth and restated by every spawn; not a
	// claim a process exists. The runner projections cannot answer it for a chat born on a connection-bound
	// provider, which made a dormant chat restart on another provider. Older chats carry "" and fall back.
	ProviderID string `json:"providerId,omitempty"`

	// ParentID and Order place the chat in the Chats tree (under a chat, a folder, or the root). The Tree
	// aggregate owns them; the chat repository fills them on read and never stores them here.
	ParentID string `json:"-"`
	Order    int    `json:"-"`
}

// ChatSelection is a model/effort pick committed atomically with the prompt it was made for. nil means
// "nothing staged"; a non-nil zero value is a real pick (provider default), hence the pointer. The
// halves travel together because valid efforts depend on the model.
type ChatSelection struct {
	Model  string
	Effort string
}
