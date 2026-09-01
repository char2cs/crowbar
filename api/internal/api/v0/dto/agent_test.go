package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The chat aggregate still carries no process facts — a chat does not own the CLI
// talking to it. The runner facts on the DTO are DERIVED at read time from the runner
// projections and handed in as a ChatRuntime; the chat itself supplies only identity,
// title and creation time.
func TestAgentChatDTOFrom_CarriesIdentityAndTitle(t *testing.T) {
	created := time.Unix(1, 0).UTC()
	got := dto.AgentChatDTOFrom(domain.Chat{
		ID:          "c1",
		WorkspaceID: "ws1",
		Title:       "a title",
		CreatedAt:   created,
	}, dto.ChatRuntime{})

	assert.Equal(t, "c1", got.ID)
	assert.Equal(t, "ws1", got.WorkspaceID)
	assert.Equal(t, "a title", got.Title)
	assert.Equal(t, created, got.CreatedAt)
}

// TestAgentChatDTOFrom_LiveRunnerIsTheLivenessAnswer proves the live join: the runner
// placed on the chat supplies liveRunnerId (which IS the liveness answer — no status
// field exists to disagree with it), its PTY, and the provider. Its provider outranks
// the history, whose last entry can still name the OUTGOING vendor mid-switch.
func TestAgentChatDTOFrom_LiveRunnerIsTheLivenessAnswer(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{
		LiveRunner: &agents.Runner{
			ID:              "run-1",
			ProviderID:      "vendor-b",
			TerminalSession: "term-1",
			CurrentChatID:   "c1",
		},
		Conversations: []agents.ChatConversation{{ChatID: "c1", ProviderID: "vendor-a"}},
	})

	assert.Equal(t, "run-1", got.LiveRunnerID)
	assert.Equal(t, "term-1", got.TerminalSessionID)
	assert.Equal(t, "vendor-b", got.ActiveProviderID)
}

// TestAgentChatDTOFrom_LiveAPIConnectionBlanksTheCompanionPTY proves the DTO's own
// honesty fix: while a non-hotswap runner has a LIVE api connection (nothing
// attached), its own TerminalSession is the disconnected companion PTY every
// api-transport spawn still forks alongside a live connection — never a real view —
// so the wire must never hand it to the frontend as if it were one. Confirmed live:
// reporting it is what let a user type into an unrelated codex session and have it
// silently promoted into its own new chat.
func TestAgentChatDTOFrom_LiveAPIConnectionBlanksTheCompanionPTY(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{
		LiveRunner: &agents.Runner{
			ID:              "run-1",
			ProviderID:      "codex",
			TerminalSession: "term-1",
			CurrentChatID:   "c1",
		},
		HasLiveAPIConnection: true,
	})

	assert.Equal(t, "run-1", got.LiveRunnerID)
	assert.Empty(t, got.TerminalSessionID, "the companion PTY must never be reported as a view")
}

// TestAgentChatDTOFrom_AttachedSessionOutranksLiveAPIConnection documents the
// priority order between the two fields even though this combination cannot occur
// in practice — attaching tears the api connection down, so a runner never carries
// both AttachedSessionID and HasLiveAPIConnection at once.
func TestAgentChatDTOFrom_AttachedSessionOutranksLiveAPIConnection(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{
		LiveRunner: &agents.Runner{
			ID:              "run-1",
			ProviderID:      "codex",
			TerminalSession: "term-1",
			CurrentChatID:   "c1",
		},
		AttachedSessionID:    "attach-1",
		HasLiveAPIConnection: true,
	})

	assert.Equal(t, "attach-1", got.TerminalSessionID)
}

// TestAgentChatDTOFrom_DormantFallsBackToLastConversation proves the fallback that keeps
// a dormant chat legible: no live runner means no runner id and no PTY, but
// activeProviderId still resolves — off the LAST conversation (history is oldest-first)
// — so the provider dropdown shows the right vendor and Resume knows who to bring back.
func TestAgentChatDTOFrom_DormantFallsBackToLastConversation(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{
		Conversations: []agents.ChatConversation{
			{ChatID: "c1", ProviderID: "vendor-a", FirstSeenAt: time.Unix(1, 0).UTC()},
			{ChatID: "c1", ProviderID: "vendor-b", FirstSeenAt: time.Unix(2, 0).UTC()},
		},
	})

	assert.Empty(t, got.LiveRunnerID)
	assert.Empty(t, got.TerminalSessionID)
	assert.Equal(t, "vendor-b", got.ActiveProviderID)
}

// TestAgentChatDTOFrom_NeverRanIsAllEmpty proves a chat no runner has ever been placed
// on derives to empty strings everywhere rather than erroring or inventing a provider.
func TestAgentChatDTOFrom_NeverRanIsAllEmpty(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{})

	assert.Empty(t, got.LiveRunnerID)
	assert.Empty(t, got.TerminalSessionID)
	assert.Empty(t, got.ActiveProviderID)
}

// TestAgentChatDTOFrom_CarriesTheRowType proves the DTO's own missing fact: a
// client reading GET .../repos/:repoId/chats could not tell a locked-branch or
// repo-home row (domain.ChatTypeBranch) apart from an ordinary chat
// (domain.ChatTypeChat) in the same list, because the wire shape dropped Type
// entirely. It is always present rather than omitted — every row this DTO ever
// serializes has a real Type, so there is no meaningful absent case to
// distinguish, unlike ParentID/Order's documented ""/0-is-meaningful pattern.
func TestAgentChatDTOFrom_CarriesTheRowType(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1", Type: domain.ChatTypeBranch},
		dto.ChatRuntime{})

	assert.Equal(t, domain.ChatTypeBranch, got.Type)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"type":"branch"`)
}

// TestAgentChatDTOList_IsNonNil keeps the envelope carrying [] rather than null, and
// proves each row derives from ITS OWN runtime: the map is keyed by chat id, and a chat
// missing from it (in neither runner projection) reads as dormant with no history.
func TestAgentChatDTOList_IsNonNil(t *testing.T) {
	got := dto.AgentChatDTOList(nil, nil)
	require.NotNil(t, got)
	assert.Empty(t, got)

	got = dto.AgentChatDTOList(
		[]domain.Chat{{ID: "c1"}, {ID: "c2"}},
		map[string]dto.ChatRuntime{
			"c1": {LiveRunner: &agents.Runner{ID: "run-1", ProviderID: "vendor-a", TerminalSession: "term-1"}},
		},
	)
	require.Len(t, got, 2)
	assert.Equal(t, "c1", got[0].ID)
	assert.Equal(t, "run-1", got[0].LiveRunnerID)
	assert.Equal(t, "term-1", got[0].TerminalSessionID)
	assert.Equal(t, "vendor-a", got[0].ActiveProviderID)

	assert.Equal(t, "c2", got[1].ID)
	assert.Empty(t, got[1].LiveRunnerID, "a chat in neither projection is dormant, not an error")
	assert.Empty(t, got[1].ActiveProviderID)
}

// TestAgentChatDetailDTOFrom_CarriesConversations proves the detail shape returns the
// chat's append-only conversation history (the successor of the deleted `segments`)
// alongside the same derived runner facts the list rows carry.
func TestAgentChatDetailDTOFrom_CarriesConversations(t *testing.T) {
	convs := []agents.ChatConversation{
		{ChatID: "c1", ProviderID: "vendor-a", SessionID: "sess-1", FirstSeenAt: time.Unix(1, 0).UTC()},
		{ChatID: "c1", ProviderID: "vendor-b", SessionID: "sess-2", FirstSeenAt: time.Unix(2, 0).UTC()},
	}
	got := dto.AgentChatDetailDTOFrom(
		domain.Chat{ID: "c1", WorkspaceID: "ws1"},
		dto.ChatRuntime{
			LiveRunner:    &agents.Runner{ID: "run-1", ProviderID: "vendor-b", TerminalSession: "term-1"},
			Conversations: convs,
		},
	)

	assert.Equal(t, "c1", got.ID)
	assert.Equal(t, "run-1", got.LiveRunnerID)
	assert.Equal(t, "term-1", got.TerminalSessionID)
	assert.Equal(t, convs, got.Conversations)
}

// TestAgentChatDetailDTOFrom_ConversationsNeverNull keeps the envelope carrying [] on a
// chat that has hosted no conversation yet, so the FE can map over it unguarded.
func TestAgentChatDetailDTOFrom_ConversationsNeverNull(t *testing.T) {
	got := dto.AgentChatDetailDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{})

	require.NotNil(t, got.Conversations)
	assert.Empty(t, got.Conversations)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"conversations":[]`)
	assert.NotContains(t, string(raw), `"segments"`)
}

// TestAgentChatDTOFrom_CarriesTheStickySelection: the chat's model/effort choice
// is durable config like the title, and the client needs it to show the picker's
// current value.
func TestAgentChatDTOFrom_CarriesTheStickySelection(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1", Model: "opus", Effort: "high"},
		dto.ChatRuntime{})

	assert.Equal(t, "opus", got.Model)
	assert.Equal(t, "high", got.Effort)
}

// TestAgentChatDTOFrom_UnselectedChatOmitsTheSelection keeps "the provider's own
// default" off the wire as an ABSENCE rather than as an empty string that a
// client might render as a selected value. Crowbar does not know what the default
// resolves to and must not imply that it does.
func TestAgentChatDTOFrom_UnselectedChatOmitsTheSelection(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{})

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"model"`)
	assert.NotContains(t, string(raw), `"effort"`)
}

// TestRegression_AgentMessageDTO_CarriesTheReportedEffort.
//
// chatlog.Turn has carried Effort since claude's turn_stop mapping began reading
// effort.level, and the projection stores it in the `effort` column — but the
// wire shape had no field, so the value was written, read back and silently
// dropped at the API boundary. Dead data, invisible to every client.
//
// The fact is the provider's own report of what it USED, which is not the chat's
// requested selection: a chat that selected nothing can still receive one.
func TestRegression_AgentMessageDTO_CarriesTheReportedEffort(t *testing.T) {
	raw, err := json.Marshal(dto.AgentMessageDTO{Sequence: 1, TurnID: "t1", Effort: "high"})

	require.NoError(t, err)
	assert.Contains(t, string(raw), `"effort":"high"`)

	bare, err := json.Marshal(dto.AgentMessageDTO{Sequence: 1, TurnID: "t1"})
	require.NoError(t, err)
	assert.NotContains(t, string(bare), `"effort"`,
		"a provider that reported no effort must produce no field, never an empty one")
}

// TestTerminalWaitDTOFrom_NotWaitingIsNil proves the zero verdict — the answer
// every chat gave before this fact existed, and the one every provider that
// declares no needles gives forever — collapses to a nil DTO rather than a
// present-and-false one.
func TestTerminalWaitDTOFrom_NotWaitingIsNil(t *testing.T) {
	got := dto.TerminalWaitDTOFrom(domain.AgentTerminalWait{})

	assert.Nil(t, got)
}

// TestTerminalWaitDTOFrom_WaitingCarriesKind proves a recognised prompt survives
// the mapping with its Kind intact.
func TestTerminalWaitDTOFrom_WaitingCarriesKind(t *testing.T) {
	got := dto.TerminalWaitDTOFrom(domain.AgentTerminalWait{
		Waiting: true,
		Kind:    domain.AgentTerminalWaitTrust,
	})

	require.NotNil(t, got)
	assert.Equal(t, domain.AgentTerminalWaitTrust, got.Kind)
}

// TestAgentChatDTOFrom_TerminalWaitOmittedWhenNotWaiting asserts on the
// MARSHALLED JSON rather than the Go struct, because the promise this feature
// makes is a wire-shape promise: an unaffected chat's JSON is byte-identical to
// what it was before TerminalWait existed. A present-but-null field would break
// that promise just as much as a wrong value would.
func TestAgentChatDTOFrom_TerminalWaitOmittedWhenNotWaiting(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{})

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"terminalWait"`)
}

// TestAgentChatDTOFrom_TerminalWaitCarriesKindWhenWaiting proves a chat whose
// runtime reports a recognised prompt carries it on the wire, kind and all.
func TestAgentChatDTOFrom_TerminalWaitCarriesKindWhenWaiting(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{
		TerminalWait: domain.AgentTerminalWait{
			Waiting: true,
			Kind:    domain.AgentTerminalWaitTrust,
		},
	})

	require.NotNil(t, got.TerminalWait)
	assert.Equal(t, domain.AgentTerminalWaitTrust, got.TerminalWait.Kind)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"terminalWait":{"kind":"workspace_trust"}`)
}

// TestAgentChatDTOFrom_UnidentifiedTerminalWaitMarshalsAsEmptyObject proves the
// unrecognised-prompt case: Waiting true with no Kind is a REAL answer — something
// is up, but the daemon has no name for it — not a missing one, so it marshals as
// a present, empty object rather than being omitted like the not-waiting case.
// Presence is the verdict; the empty kind is the honest detail.
func TestAgentChatDTOFrom_UnidentifiedTerminalWaitMarshalsAsEmptyObject(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{
		TerminalWait: domain.AgentTerminalWait{Waiting: true},
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"terminalWait":{}`)
}
