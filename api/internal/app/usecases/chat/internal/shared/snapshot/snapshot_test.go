package snapshot_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	"github.com/char2cs/crowbar/api/internal/domain"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type reader struct {
	chats map[string]domain.Chat
	live  []agents.Runner
}

func (r *reader) GetChat(_ context.Context, id string) (domain.Chat, error) {
	c, ok := r.chats[id]
	if !ok {
		return domain.Chat{}, errors.New("not found")
	}
	return c, nil
}

func (r *reader) AllLive(context.Context) ([]agents.Runner, error) { return r.live, nil }

type runtime struct {
	mu     sync.Mutex
	phases map[string]string
}

func (r *runtime) Phase(chatID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.phases[chatID]
}
func (*runtime) TerminalWait(string) domain.AgentTerminalWait  { return domain.AgentTerminalWait{} }
func (*runtime) AttachedTerminalSession(string) (string, bool) { return "", false }
func (*runtime) Session(string) domain.AgentSession            { return domain.AgentSession{} }

type recorder struct {
	mu     sync.Mutex
	frames []snapshot.Frame
}

func (r *recorder) publish(f snapshot.Frame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, f)
}

func (r *recorder) last(t *testing.T) snapshot.Frame {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.NotEmpty(t, r.frames)
	return r.frames[len(r.frames)-1]
}

func newOwner(t *testing.T, rd *reader) (*snapshot.Snapshots, *recorder, *runtime) {
	t.Helper()
	s := snapshot.New(1000)
	rec := &recorder{}
	rt := &runtime{phases: map[string]string{}}
	s.SetPublish(rec.publish)
	s.Bind(rd, rt)
	return s, rec, rt
}

var t0 = time.Unix(1_000, 0).UTC()

func runnerOn(id, chatID string, at time.Time) agents.Runner {
	return agents.Runner{ID: id, CurrentChatID: chatID, ProviderID: "claude", StartedAt: at}
}

// A6: every answer about a chat carries a version, and a later answer carries
// a larger one — whatever produced it.
func TestSnapshots_EveryChangeCarriesALargerVersion(t *testing.T) {
	ctx := context.Background()
	s, rec, _ := newOwner(t, &reader{chats: map[string]domain.Chat{"c1": {ID: "c1"}}})

	first, err := s.Get(ctx, "c1")
	require.NoError(t, err)
	s.ApplyChat(ctx, domain.Chat{ID: "c1", Working: true}, 2, "turn_started", false)
	started := rec.last(t).Snapshot
	s.ApplyRunner(ctx, runnerOn("r1", "c1", t0), agents.Runner{}, 1, "started")
	placed := rec.last(t).Snapshot

	assert.Greater(t, started.Version, first.Version)
	assert.Greater(t, placed.Version, started.Version)
	assert.True(t, placed.Chat.Working, "the chat half comes from the chat event, not re-read")
	require.NotNil(t, placed.Live)
	assert.Equal(t, "r1", placed.Live.ID)

	again, err := s.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, placed.Version, again.Version, "a read never advances the version")
}

// The root of the "dead pane over a live CLI" class: a frame must never
// describe a chat older than an event already announced. The chat's working
// flag is the command-side fold carried on the event, so a read model that has
// not caught up cannot put an older answer under a newer version.
func TestSnapshots_AFrameNeverLagsTheEventItAnnounces(t *testing.T) {
	ctx := context.Background()
	rd := &reader{chats: map[string]domain.Chat{"c1": {ID: "c1", Working: false}}}
	s, rec, _ := newOwner(t, rd)

	s.ApplyChat(ctx, domain.Chat{ID: "c1", Working: true}, 5, "turn_started", false)
	assert.True(t, rec.last(t).Snapshot.Chat.Working)

	s.ApplyChat(ctx, domain.Chat{ID: "c1", Working: false}, 4, "late", false)
	assert.True(t, rec.last(t).Snapshot.Chat.Working, "an older aggregate version never overwrites a newer one")
}

// A moved runner changes TWO chats; both are republished, the one it left
// without it.
func TestSnapshots_AMoveRepublishesTheChatLeftAndTheChatEntered(t *testing.T) {
	ctx := context.Background()
	rd := &reader{
		chats: map[string]domain.Chat{"a": {ID: "a"}, "b": {ID: "b"}},
		live:  []agents.Runner{runnerOn("r1", "a", t0)},
	}
	s, rec, _ := newOwner(t, rd)

	moved := runnerOn("r1", "b", t0)
	moved.CurrentSessionSince = t0.Add(time.Second)
	s.ApplyRunner(ctx, moved, runnerOn("r1", "a", t0), 3, "moved")

	rec.mu.Lock()
	frames := append([]snapshot.Frame(nil), rec.frames...)
	rec.mu.Unlock()
	require.Len(t, frames, 2)
	assert.Equal(t, "b", frames[0].Snapshot.Chat.ID)
	assert.Equal(t, "moved", frames[0].Kind)
	require.NotNil(t, frames[0].Snapshot.Live)
	assert.Equal(t, snapshot.PhaseLive, frames[0].Snapshot.Phase)
	assert.Equal(t, "a", frames[1].Snapshot.Chat.ID)
	assert.Nil(t, frames[1].Snapshot.Live, "the chat it left is dormant")
	assert.Equal(t, snapshot.PhaseDormant, frames[1].Snapshot.Phase)
	assert.Equal(t, "r1", frames[1].RunnerID)
}

// A runner taken off its chat, and its exit after that, both reach the chat it
// held under their own kinds: a client lets go of a displaced runner at once.
func TestSnapshots_ADisplacedRunnerAndItsExitReachTheChatItLeft(t *testing.T) {
	ctx := context.Background()
	s, rec, _ := newOwner(t, &reader{chats: map[string]domain.Chat{"c1": {ID: "c1"}}})
	s.ApplyRunner(ctx, runnerOn("r1", "c1", t0), agents.Runner{}, 1, "started")

	s.ApplyRunner(ctx, runnerOn("r1", "", t0), runnerOn("r1", "c1", t0), 2, "displaced")
	displaced := rec.last(t)
	assert.Equal(t, "displaced", displaced.Kind)
	assert.Equal(t, "c1", displaced.Snapshot.Chat.ID)
	assert.Nil(t, displaced.Snapshot.Live)

	exited := runnerOn("r1", "", t0)
	exitedAt := t0.Add(time.Minute)
	exited.ExitedAt = &exitedAt
	s.ApplyRunner(ctx, exited, runnerOn("r1", "", t0), 3, "exited")
	last := rec.last(t)
	assert.Equal(t, "exited", last.Kind)
	assert.Equal(t, "c1", last.Snapshot.Chat.ID)
	assert.Equal(t, "r1", last.RunnerID)
}

// Two runners claiming one chat: the newest arrival is the live one — the same
// answer the read model's single-chat query gives.
func TestSnapshots_TheNewestArrivalIsLive(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newOwner(t, &reader{
		chats: map[string]domain.Chat{"c1": {ID: "c1"}},
		live:  []agents.Runner{runnerOn("old", "c1", t0), runnerOn("new", "c1", t0.Add(time.Minute))},
	})
	got, err := s.Get(ctx, "c1")
	require.NoError(t, err)
	require.NotNil(t, got.Live)
	assert.Equal(t, "new", got.Live.ID)
}

// An exit removes the runner; a late, older event of the same runner cannot
// bring it back.
func TestSnapshots_AnExitedRunnerStaysGone(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newOwner(t, &reader{chats: map[string]domain.Chat{"c1": {ID: "c1"}}})
	s.ApplyRunner(ctx, runnerOn("r1", "c1", t0), agents.Runner{}, 1, "started")
	exited := runnerOn("r1", "c1", t0)
	exitedAt := t0.Add(time.Minute)
	exited.ExitedAt = &exitedAt
	s.ApplyRunner(ctx, exited, runnerOn("r1", "c1", t0), 3, "exited")

	got, err := s.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Nil(t, got.Live)
}

// The phase a lifecycle operation sets rides the snapshot; with none set it is
// derived from placement.
func TestSnapshots_PhaseIsTheOperationsElseDerived(t *testing.T) {
	ctx := context.Background()
	s, rec, rt := newOwner(t, &reader{chats: map[string]domain.Chat{"c1": {ID: "c1"}}})

	got, err := s.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, snapshot.PhaseDormant, got.Phase)

	rt.mu.Lock()
	rt.phases["c1"] = snapshot.PhaseStarting
	rt.mu.Unlock()
	s.Touch(ctx, "c1")
	assert.Equal(t, snapshot.PhaseStarting, rec.last(t).Snapshot.Phase)
	assert.Equal(t, snapshot.KindSnapshot, rec.last(t).Kind)
}

// A7: deleting a chat removes its entry; the delete frame still carries a
// version larger than any before it.
func TestSnapshots_ADeletedChatLeavesNothingBehind(t *testing.T) {
	ctx := context.Background()
	s, rec, _ := newOwner(t, &reader{chats: map[string]domain.Chat{"c1": {ID: "c1"}}})
	s.ApplyChat(ctx, domain.Chat{ID: "c1"}, 1, "created", false)
	before := rec.last(t).Snapshot.Version

	s.ApplyChat(ctx, domain.Chat{ID: "c1"}, 2, "deleted", true)

	f := rec.last(t)
	assert.True(t, f.Deleted)
	assert.Greater(t, f.Snapshot.Version, before)
	assert.Zero(t, s.Len(), "a deleted chat holds no entry")
}

// A chat nobody can read is not held: an unknown id answers an error and
// leaves no entry behind.
func TestSnapshots_AnUnknownChatIsNotHeld(t *testing.T) {
	s, _, _ := newOwner(t, &reader{chats: map[string]domain.Chat{}})
	_, err := s.Get(context.Background(), "ghost")
	require.Error(t, err)
	assert.Zero(t, s.Len())
}

// A restarted daemon's versions are larger than any the previous one issued,
// so a client holding an old version accepts the new daemon's answers.
func TestSnapshots_VersionsSurviveARestart(t *testing.T) {
	ctx := context.Background()
	rd := &reader{chats: map[string]domain.Chat{"c1": {ID: "c1"}}}
	before := snapshot.NewAtBoot()
	before.Bind(rd, &runtime{phases: map[string]string{}})
	for range 100 {
		before.Touch(ctx, "c1")
	}
	old, err := before.Get(ctx, "c1")
	require.NoError(t, err)

	time.Sleep(time.Millisecond)
	after := snapshot.NewAtBoot()
	after.Bind(rd, &runtime{phases: map[string]string{}})
	fresh, err := after.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Greater(t, fresh.Version, old.Version)
}
