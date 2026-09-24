package agents_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// TestActiveProviderID_ProviderThatNeverBoundAConversationIsFoundViaInterruption is
// the failing case this function exists to fix: a chat's only conversation row
// names an OLDER provider, and the provider actually running when the chat went
// dormant left no row at all (it binds via its own connection identity) — only a
// durable switch interruption naming it. The interruption is newer, so it must win.
func TestActiveProviderID_ProviderThatNeverBoundAConversationIsFoundViaInterruption(t *testing.T) {
	conversations := []agents.ChatConversation{
		{ChatID: "c1", ProviderID: "vendor-a", LastActiveAt: time.Unix(1, 0).UTC()},
	}
	interruptions := []domain.ActivityInterruption{
		{ChatID: "c1", Kind: agents.InterruptProviderSwitched, Detail: "vendor-b", At: time.Unix(2, 0).UTC()},
	}

	providerID, found := agents.ActiveProviderID(conversations, interruptions, nil)

	assert.True(t, found)
	assert.Equal(t, "vendor-b", providerID, "the newer switch interruption must win over the older conversation row")
}

// TestActiveProviderID_AnOlderInterruptionLosesToANewerConversation is the mirror:
// a conversation row that postdates the last switch interruption (the chat later
// bound a real conversation on the current provider) must win.
func TestActiveProviderID_AnOlderInterruptionLosesToANewerConversation(t *testing.T) {
	conversations := []agents.ChatConversation{
		{ChatID: "c1", ProviderID: "vendor-b", LastActiveAt: time.Unix(2, 0).UTC()},
	}
	interruptions := []domain.ActivityInterruption{
		{ChatID: "c1", Kind: agents.InterruptProviderSwitched, Detail: "vendor-a", At: time.Unix(1, 0).UTC()},
	}

	providerID, found := agents.ActiveProviderID(conversations, interruptions, nil)

	assert.True(t, found)
	assert.Equal(t, "vendor-b", providerID)
}

// TestActiveProviderID_ScansForMaxLastActiveAtNotSliceOrder preserves the
// pre-existing behaviour: a chat switched back to a provider it already ran
// re-activates that provider's own EARLIER row rather than minting a new one, so
// slice position must never be trusted over LastActiveAt.
func TestActiveProviderID_ScansForMaxLastActiveAtNotSliceOrder(t *testing.T) {
	conversations := []agents.ChatConversation{
		{ChatID: "c1", ProviderID: "vendor-a", LastActiveAt: time.Unix(3, 0).UTC()},
		{ChatID: "c1", ProviderID: "vendor-b", LastActiveAt: time.Unix(1, 0).UTC()},
	}

	providerID, found := agents.ActiveProviderID(conversations, nil, nil)

	assert.True(t, found)
	assert.Equal(t, "vendor-a", providerID, "must scan for the max LastActiveAt, not take the last slice element")
}

// TestActiveProviderID_IgnoresInterruptionsOfOtherKinds proves only
// InterruptProviderSwitched entries are read — a permission wait or a compaction
// marker must never be mistaken for a provider answer.
func TestActiveProviderID_IgnoresInterruptionsOfOtherKinds(t *testing.T) {
	conversations := []agents.ChatConversation{
		{ChatID: "c1", ProviderID: "vendor-a", LastActiveAt: time.Unix(1, 0).UTC()},
	}
	interruptions := []domain.ActivityInterruption{
		{ChatID: "c1", Kind: agents.InterruptCompaction, Detail: "vendor-b", At: time.Unix(9, 0).UTC()},
	}

	providerID, found := agents.ActiveProviderID(conversations, interruptions, nil)

	assert.True(t, found)
	assert.Equal(t, "vendor-a", providerID)
}

// TestActiveProviderID_NothingEverRanIsNotFound proves the "no provider has ever
// run" case is distinguishable from every real answer: empty inputs answer
// found=false, never a zero-value provider id.
func TestActiveProviderID_NothingEverRanIsNotFound(t *testing.T) {
	providerID, found := agents.ActiveProviderID(nil, nil, nil)

	assert.False(t, found)
	assert.Empty(t, providerID)
}

// TestRegression_ResolveProviderID_FallsBackToTheChatsOwnStoredChoice is the
// case NEITHER runner projection can answer: a chat BORN on a provider that
// binds by its own connection identity. It writes no conversation row, and a
// chat born on it was never switched, so there is no provider_switched marker
// either. ActiveProviderID answers found=false — and every caller that treated
// that as "no provider has ever run here" let the chat be converted to whatever
// happened to be first in the catalogue.
func TestRegression_ResolveProviderID_FallsBackToTheChatsOwnStoredChoice(t *testing.T) {
	providerID, found := agents.ResolveProviderID(nil, nil, nil, "codex")

	assert.True(t, found)
	assert.Equal(t, "codex", providerID)
}

// TestResolveProviderID_ProjectionsWinOverTheStoredChoice pins the precedence:
// the stored choice is LAST, answering only what the projections are blind to.
func TestResolveProviderID_ProjectionsWinOverTheStoredChoice(t *testing.T) {
	conversations := []agents.ChatConversation{
		{ChatID: "c1", ProviderID: "vendor-b", LastActiveAt: time.Unix(1, 0).UTC()},
	}

	providerID, found := agents.ResolveProviderID(conversations, nil, nil, "vendor-a")

	assert.True(t, found)
	assert.Equal(t, "vendor-b", providerID)
}

// TestResolveProviderID_NothingAnywhereIsStillNotFound keeps the "nothing has
// ever run here" answer reachable: a chat minted before this field exists
// carries "" and must stay distinguishable from a real provider id.
func TestResolveProviderID_NothingAnywhereIsStillNotFound(t *testing.T) {
	providerID, found := agents.ResolveProviderID(nil, nil, nil, "")

	assert.False(t, found)
	assert.Empty(t, providerID)
}

// TestRegression_ResolveProviderID_RecoveredFromPlacementHistoryAlone is the
// user's actual bug in one function: a chat with NO conversation row (its
// provider announces none), NO switch interruption (it was never switched) and NO
// stored provider (it was minted before that field existed) — the exact shape
// every chat on a populated machine has. The daemon recorded a runner placement
// on it every time a CLI started, and this is the source that reads it.
//
// Without placements this call answers found=false, which is what surfaced as
// "agentrunner not found" on resume.
func TestRegression_ResolveProviderID_RecoveredFromPlacementHistoryAlone(t *testing.T) {
	placements := []agents.ChatPlacement{
		{ChatID: "c1", ProviderID: "vendor-b", LastPlacedAt: time.Unix(5, 0).UTC()},
	}

	providerID, found := agents.ResolveProviderID(nil, nil, placements, "")

	assert.True(t, found)
	assert.Equal(t, "vendor-b", providerID)
}

// TestActiveProviderID_PlacementScansForMaxLastPlacedAtNotSliceOrder holds
// placements to the same rule conversations obey: the answer is the NEWEST
// arrival, never the last slice element, because a provider re-placed on a chat
// it already ran keeps its original row.
func TestActiveProviderID_PlacementScansForMaxLastPlacedAtNotSliceOrder(t *testing.T) {
	placements := []agents.ChatPlacement{
		{ChatID: "c1", ProviderID: "vendor-a", LastPlacedAt: time.Unix(7, 0).UTC()},
		{ChatID: "c1", ProviderID: "vendor-b", LastPlacedAt: time.Unix(2, 0).UTC()},
	}

	providerID, found := agents.ActiveProviderID(nil, nil, placements)

	assert.True(t, found)
	assert.Equal(t, "vendor-a", providerID)
}

// TestActiveProviderID_ANewerPlacementWinsOverAnOlderConversationAndInterruption
// pins where placements sit in the scan: they are evidence of the same kind as
// the other two — Crowbar's own record of pointing a CLI at the chat — so a
// provider that demonstrably arrived LAST is the answer, whatever the older
// sources say.
func TestActiveProviderID_ANewerPlacementWinsOverAnOlderConversationAndInterruption(t *testing.T) {
	conversations := []agents.ChatConversation{
		{ChatID: "c1", ProviderID: "vendor-a", LastActiveAt: time.Unix(1, 0).UTC()},
	}
	interruptions := []domain.ActivityInterruption{
		{ChatID: "c1", Kind: agents.InterruptProviderSwitched, Detail: "vendor-b", At: time.Unix(2, 0).UTC()},
	}
	placements := []agents.ChatPlacement{
		{ChatID: "c1", ProviderID: "vendor-a", LastPlacedAt: time.Unix(1, 0).UTC()},
		{ChatID: "c1", ProviderID: "vendor-c", LastPlacedAt: time.Unix(3, 0).UTC()},
	}

	providerID, found := agents.ActiveProviderID(conversations, interruptions, placements)

	assert.True(t, found)
	assert.Equal(t, "vendor-c", providerID)
}

// TestActiveProviderID_AnOlderPlacementLosesToANewerConversation is the mirror,
// and it is what keeps the hard-won conversation and interruption orderings
// intact: a placement that predates the newest conversation row changes nothing.
// Adding placements may only ever ADD answers, never rewrite one the other
// sources already got right.
func TestActiveProviderID_AnOlderPlacementLosesToANewerConversation(t *testing.T) {
	conversations := []agents.ChatConversation{
		{ChatID: "c1", ProviderID: "vendor-b", LastActiveAt: time.Unix(9, 0).UTC()},
	}
	placements := []agents.ChatPlacement{
		{ChatID: "c1", ProviderID: "vendor-a", LastPlacedAt: time.Unix(4, 0).UTC()},
	}

	providerID, found := agents.ActiveProviderID(conversations, nil, placements)

	assert.True(t, found)
	assert.Equal(t, "vendor-b", providerID)
}

// TestActiveProviderID_APlacementTieDoesNotDisplaceTheConversation pins the
// tie-break, which is the safety property of putting placements last in the
// scan: a runner's placement and the conversation it bound carry the SAME
// instant, so the scan must keep the incumbent rather than flip on equal
// evidence.
func TestActiveProviderID_APlacementTieDoesNotDisplaceTheConversation(t *testing.T) {
	at := time.Unix(6, 0).UTC()
	conversations := []agents.ChatConversation{
		{ChatID: "c1", ProviderID: "vendor-b", LastActiveAt: at},
	}
	placements := []agents.ChatPlacement{
		{ChatID: "c1", ProviderID: "vendor-a", LastPlacedAt: at},
	}

	providerID, found := agents.ActiveProviderID(conversations, nil, placements)

	assert.True(t, found)
	assert.Equal(t, "vendor-b", providerID)
}

// TestResolveProviderID_PlacementHistoryWinsOverTheStoredChoice keeps stored
// LAST even now that a third projection exists: the scan is live-reported
// evidence of what actually ran, and the chat's durable field is only what it was
// born as — a chat switched away from its birth vendor and then stopped must come
// back as what was running, not as what it started life on.
func TestResolveProviderID_PlacementHistoryWinsOverTheStoredChoice(t *testing.T) {
	placements := []agents.ChatPlacement{
		{ChatID: "c1", ProviderID: "vendor-b", LastPlacedAt: time.Unix(3, 0).UTC()},
	}

	providerID, found := agents.ResolveProviderID(nil, nil, placements, "vendor-a")

	assert.True(t, found)
	assert.Equal(t, "vendor-b", providerID)
}
