package dto

import "time"

// AgentChatEvent is the wire frame pushed on the agent-chat lifecycle WebSocket
// (GET .../repos/:repoId/chats/ws, and still GET .../home/chats/ws): the thing
// that changed, the workspace it belongs to, and the lifecycle kind — chat
// kinds (created/turn_started/turn_stopped/title_set/placement_set/deleted),
// runner kinds (started/exited), and folder
// kinds (folder_created/folder_updated/folder_deleted), all of which ride this
// same feed. It carries no snapshot; the stream is a bare event feed, not a
// full-state resource stream. WorkspaceID rides along on the wire frame and
// still scopes the home mount's feed (agentChatDef's wsId Filter, keyed off
// its injected :wsId). The repo-scoped mount names no workspace in its URL, so
// that Filter resolves inactive there and scopes nothing; what scopes it is
// RepoID below, against the mount's own :repoId, together with ProjectID
// against its :projectId. Before RepoID existed the wsId Filter was this
// stream's only scoping mechanism and a repo-scoped client received every OTHER
// repo's chat events too; before ProjectID existed, every REPO-LESS frame — a
// folder row, a root bubble, and every chat in a project HOME — still did.
type AgentChatEvent struct {
	ChatID      string `json:"chatId"`
	WorkspaceID string `json:"workspaceId"`
	// RepoID is the repo the frame's row actually runs in, resolved server-side
	// from the row's own ground: its workspace when it has one, and — for a
	// BUBBLE, which has none — the workspace its cwd walk lands on (model spec
	// §3.2). It is what scopes the repo-scoped mount (agentChatDef's repoId
	// Filter). It is on the frame because a row's repo is DERIVED, never
	// stored, so the only place it can be answered is where the walk runs.
	//
	// EMPTY means "this row has no repo to be held to" — a folder row, a
	// bubble whose ancestry owns no workspace, or ANY row in a project home,
	// which owns no repo — and such a frame reaches every subscriber of the
	// same PROJECT rather than none (see matchScopeOrUnscoped).
	RepoID string `json:"repoId,omitempty"`
	// ProjectID is the project the frame's row runs in, resolved from the same
	// workspace row RepoID is and in the same pass. It is what bounds a
	// REPO-LESS frame, which RepoID by construction cannot: a project home owns
	// no repo, so every frame its chats emit — a lifecycle edge, a plan, the
	// streamed text of a turn — carried an empty RepoID and was therefore
	// forwarded to every repo-scoped subscriber in the daemon, across projects.
	// Holding those frames to an exact repo is not an option (it would drop the
	// folder rows and root bubbles that legitimately have none), so they are
	// held to the project instead: the narrowest scope that still keeps them.
	//
	// EMPTY means the row resolved nothing at all — its placement projection has
	// not caught up, or its whole ancestry owns no workspace — and such a frame
	// still reaches everyone, unchanged.
	ProjectID string `json:"projectId,omitempty"`
	Kind      string `json:"kind"`
	// RunnerID names the vendor-CLI process the frame is about, and is set ONLY on
	// the agent-RUNNER kinds (started/exited), which ride this same
	// workspace-scoped feed rather than a second socket. It is empty on the chat
	// kinds, which are about the chat itself and name no process.
	RunnerID string `json:"runnerId,omitempty"`
	// FolderID names the CHAT FOLDER a frame is about, and is set ONLY on the
	// folder kinds (folder_created/folder_updated/folder_deleted), which ride this
	// same workspace-scoped feed rather than a second socket — the same reason the
	// runner kinds do.
	//
	// It carries the id and nothing else, deliberately. This stream is a bare event
	// feed, not a full-state resource stream: it has no snapshot, so a client
	// cannot hold folders from it alone and must read the list anyway. Putting the
	// row on the frame would create a second way to learn a folder's placement,
	// and the two would disagree the moment a frame was dropped. A folder frame
	// therefore means exactly "re-read this workspace's folders" — which is also
	// what a reconnect does, so the outage path and the live path repair
	// identically.
	FolderID string `json:"folderId,omitempty"`
	// ClientRequestID names one prompt the browser is still holding in its pending
	// queue, and is set ONLY on the prompt_settled kind.
	//
	// The queue resolves an item when the prompt it sent turns up in the ledger as
	// a user message. A provider's own built-in command never produces one — the
	// CLI handles it and announces nothing — so without this frame the item waits
	// forever on evidence that is not coming.
	ClientRequestID string `json:"clientRequestId,omitempty"`

	// Worktree rides the `worktree_state` kind and nothing else: the git state of
	// the worktree ChatID owns, as of this event.
	//
	// It is on the frame rather than refetched for the same reason Working and
	// TerminalWait are — the fact IS the change, and a client that had to ask for
	// it would repaint a round trip after the diff counts moved. It is also what
	// makes the chat feed a complete substitute for the workspace stream a client
	// used to watch alongside it (spec §5): the same push site serves both, so a
	// chat-scoped subscriber and a workspace-scoped one learn of a git change in
	// the same instant and from the same bytes.
	Worktree *ChatWorktreeDTO `json:"worktree,omitempty"`

	// PromptConsumed rides the prompt_settled kind beside ClientRequestID and says
	// whether anything actually proved the provider took that prompt.
	//
	// True is a provider built-in that demonstrably ran — a `/compact` the CLI
	// handled itself, announcing nothing. False is every other settle: the launch
	// failed, the CLI exited, or the provider went idle without taking the prompt.
	//
	// A client cannot derive it — neither case produced a turn — and discarding its
	// queued copy on false would lose words the provider never read. Discard on
	// true; keep the text and surface a failure on false.
	//
	// Omitempty makes the SAFE reading the default: a frame with the field absent
	// preserves the text rather than dropping it.
	PromptConsumed bool `json:"promptConsumed,omitempty"`

	// Message is an assistant message still being produced, on the message_delta
	// kind and nowhere else.
	//
	// It is deliberately NOT in the ledger. A message that is still growing is not
	// a record of anything yet — it is a view — and writing every increment to
	// durable storage would mean roughly 1.4 writes per second per streaming chat
	// to persist text that is replaced a moment later. So the partial travels on
	// the live feed only, and the ledger gets the message once, when it is done.
	Message *AgentStreamingMessageDTO `json:"message,omitempty"`
	// Plan is the agent's running to-do list, on the `plan` kind.
	Plan []AgentPlanStepDTO `json:"plan,omitempty"`
	// Chat is the chat's full snapshot as of this frame, on every frame the
	// snapshot owner publishes — and Version its version. A client applies it
	// only when Version is newer than the one it holds; it never refetches.
	Chat    *AgentChatDTO `json:"chat,omitempty"`
	Version int64         `json:"version,omitempty"`

	// Telemetry is the provider's newest usage report, on the `telemetry` kind.
	// Pushed as it arrives (about once a turn) so no client polls for it.
	Telemetry *AgentTelemetryDTO `json:"telemetry,omitempty"`

	// Choices is every prompt the chat is blocked on, on the `choice` kind.
	Choices []AgentChoiceDTO `json:"choices,omitempty"`
}

// AgentPlanStepDTO is one entry of the agent's running plan.
type AgentPlanStepDTO struct {
	Text string `json:"text"`
	// Status is CROWBAR'S word — pending, active or done — translated from the
	// provider's own vocabulary by its descriptor, never by a client. An unknown
	// value is still a step and must render unstyled rather than be dropped.
	Status string `json:"status"`
}

// AgentStreamingMessageDTO is one assistant message as far as it has been said.
type AgentStreamingMessageDTO struct {
	// StartedAt is when this message's first text arrived — the same instant the
	// ledger will stamp the recorded row with — so a client orders the live bubble
	// against tool rows exactly as the record will. Absent for the live-only kinds.
	StartedAt time.Time `json:"startedAt,omitzero"`
	// ID is the provider's own message identity, so a client can tell a message
	// that is still growing from the next one starting.
	ID string `json:"id"`
	// Text is everything said SO FAR, not the newest increment. A client that
	// missed a frame is therefore correct again on the next one, with no
	// reassembly and no gap detection of its own.
	Text string `json:"text"`
	// Kind names WHICH stream this text belongs to. Absent (the default) is the
	// agent's answer — the stream that existed before there was more than one, and
	// the only one that is ever recorded in the ledger. "reasoning" is the agent
	// thinking on the way there: live-only, dropped when the turn ends, and
	// rendered as a thought rather than as the reply.
	//
	// Omitempty deliberately: an answer frame is byte-identical to what every
	// client already parses.
	Kind string `json:"kind,omitempty"`
}

// AgentChatKindPromptSettled announces that a prompt Crowbar delivered is OVER
// without having produced a turn, so nothing is still owed on it.
//
// It is emitted at most once per delivery, and only for the deliveries that need
// it: an ordinary prompt is resolved by its own user message arriving in the
// ledger and never reaches this path.
const AgentChatKindPromptSettled = "prompt_settled"

// AgentChatKindMessageDelta announces that an assistant message has grown.
//
// It carries the message SO FAR rather than the increment, so a dropped frame
// self-heals on the next one. It stops when the message is complete: the message
// then exists in the ledger, and the ledger is what the chat reads.
const AgentChatKindMessageDelta = "message_delta"

// AgentChatKindWorktreeState announces that the git state of the worktree a
// chat OWNS has moved: a commit, a file edited, a PR opened, a lock taken.
//
// It is a CHAT kind — it names the row a client draws, carries no runner id,
// and is emitted from the one place a workspace frame is already built, so the
// chat-scoped feed and the workspace-scoped stream can never disagree about the
// same branch. A chat that owns no worktree never produces one.
const AgentChatKindWorktreeState = "worktree_state"

// AgentChatKindPlan announces that the agent restated its own to-do list for the
// turn. The list arrives WHOLESALE — the newest one is the entire truth, so a
// client that missed a frame is correct again on the next one, and there is
// nothing to merge. Never stored: a plan for a turn in progress is a view of it.
const AgentChatKindPlan = "plan"

// AgentChatKindTelemetry announces the provider's newest usage report for the
// chat — context, rate limits, cost — carried whole on the frame.
const AgentChatKindTelemetry = "telemetry"

// AgentChatKindCompactionStarted and AgentChatKindCompactionStopped announce
// the live compact_pre/compact_post edge. Two kinds, no extra payload, same
// shape as turn_started/turn_stopped: the kind IS the answer.
const (
	AgentChatKindCompactionStarted = "compaction_started"
	AgentChatKindCompactionStopped = "compaction_stopped"
)
