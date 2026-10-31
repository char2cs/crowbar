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
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type reader struct {
	chats map[string]domain.Chat
	// ledger answers ViewLedger; nil reads every chat's ledger as empty.
	ledger func(chatID string) (ledger.State, error)
	// released runs as ViewLedger releases the chat's writer.
	released func()
	// gets and batches count the one-chat and many-chat reads.
	gets, batches int
}

func (r *reader) ViewLedger(_ context.Context, chatID string, fn func(ledger.State) error) error {
	st := ledger.State{ChatID: chatID}
	if r.ledger != nil {
		var err error
		if st, err = r.ledger(chatID); err != nil {
			return err
		}
	}
	if err := fn(st); err != nil {
		return err
	}
	if r.released != nil {
		r.released()
	}
	return nil
}

func (r *reader) GetChats(_ context.Context, ids []string) ([]domain.Chat, error) {
	r.batches++
	var out []domain.Chat
	for _, id := range ids {
		if c, ok := r.chats[id]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *reader) GetChat(_ context.Context, id string) (domain.Chat, error) {
	r.gets++
	c, ok := r.chats[id]
	if !ok {
		return domain.Chat{}, errors.New("not found")
	}
	return c, nil
}

type runtime struct {
	mu     sync.Mutex
	phases map[string]string
	live   map[string]agents.Runner
}

func (r *runtime) place(runner agents.Runner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.live[runner.CurrentChatID] = runner
}

func (r *runtime) vacate(chatID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.live, chatID)
}

func (r *runtime) LiveRunner(chatID string) (agents.Runner, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	live, ok := r.live[chatID]
	return live, ok
}

func (r *runtime) Phase(chatID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.phases[chatID]
}
func (*runtime) TerminalWait(string) domain.AgentTerminalWait  { return domain.AgentTerminalWait{} }
func (*runtime) AttachedTerminalSession(string) (string, bool) { return "", false }

func newRuntime() *runtime {
	return &runtime{phases: map[string]string{}, live: map[string]agents.Runner{}}
}

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
	rt := newRuntime()
	s.SetPublish(rec.publish)
	s.Bind(rd, rt)
	return s, rec, rt
}

var t0 = time.Unix(1_000, 0).UTC()

// liveR1 is runner r1, live on chat c1.
var liveR1 = agents.Runner{ID: "r1", CurrentChatID: "c1", ProviderID: "claude", StartedAt: t0}

// A6: every answer about a chat carries a version, and a later answer carries
// a larger one — whatever produced it.
func TestSnapshots_EveryChangeCarriesALargerVersion(t *testing.T) {
	ctx := context.Background()
	s, rec, rt := newOwner(t, &reader{chats: map[string]domain.Chat{"c1": {ID: "c1"}}})

	first, err := s.Get(ctx, "c1")
	require.NoError(t, err)
	s.ApplyChat(ctx, domain.Chat{ID: "c1", Title: "t"}, 2, "title_set", false)
	started := rec.last(t).Snapshot
	rt.place(liveR1)
	s.ApplyRunner(ctx, "c1", "r1", "started")
	placed := rec.last(t).Snapshot

	assert.Greater(t, started.Version, first.Version)
	assert.Greater(t, placed.Version, started.Version)
	assert.Equal(t, "t", placed.Chat.Title, "the chat half comes from the chat event, not re-read")
	require.NotNil(t, placed.Live)
	assert.Equal(t, "r1", placed.Live.ID)

	again, err := s.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, placed.Version, again.Version, "a read never advances the version")
}

// A frame must never describe a chat older than an event already announced:
// a read model that has not caught up cannot put an older answer under a newer
// version.
func TestSnapshots_AFrameNeverLagsTheEventItAnnounces(t *testing.T) {
	ctx := context.Background()
	rd := &reader{chats: map[string]domain.Chat{"c1": {ID: "c1", Title: "old"}}}
	s, rec, _ := newOwner(t, rd)

	s.ApplyChat(ctx, domain.Chat{ID: "c1", Title: "new"}, 5, "title_set", false)
	assert.Equal(t, "new", rec.last(t).Snapshot.Chat.Title)

	s.ApplyChat(ctx, domain.Chat{ID: "c1", Title: "old"}, 4, "late", false)
	assert.Equal(t, "new", rec.last(t).Snapshot.Chat.Title, "an older aggregate version never overwrites a newer one")
}

// A runner's start and exit reach its chat under their own kinds, naming the
// runner, and the live runner is whatever the supervisor holds at the time.
func TestSnapshots_ARunnersStartAndExitReachItsChat(t *testing.T) {
	ctx := context.Background()
	s, rec, rt := newOwner(t, &reader{chats: map[string]domain.Chat{"c1": {ID: "c1"}}})
	rt.place(liveR1)
	s.ApplyRunner(ctx, "c1", "r1", "started")
	started := rec.last(t)
	require.NotNil(t, started.Snapshot.Live)
	assert.Equal(t, snapshot.PhaseLive, started.Snapshot.Phase)

	rt.vacate("c1")
	s.ApplyRunner(ctx, "c1", "r1", "exited")

	exited := rec.last(t)
	assert.Equal(t, "exited", exited.Kind)
	assert.Equal(t, "r1", exited.RunnerID)
	assert.Nil(t, exited.Snapshot.Live)
	assert.Equal(t, snapshot.PhaseDormant, exited.Snapshot.Phase)
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

// Events are delivered concurrently, so one older than the delete can land
// after it — against a read model that has not caught up either. The chat
// stays deleted.
func TestSnapshots_AnEventLandingAfterTheDeleteDoesNotResurrectTheChat(t *testing.T) {
	ctx := context.Background()
	s, rec, _ := newOwner(t, &reader{chats: map[string]domain.Chat{"c1": {ID: "c1"}}})
	s.ApplyChat(ctx, domain.Chat{ID: "c1"}, 1, "created", false)
	s.ApplyRunner(ctx, "c1", "r1", "started")
	s.ApplyChat(ctx, domain.Chat{ID: "c1"}, 3, "deleted", true)
	deleted := len(rec.frames)

	s.ApplyChat(ctx, domain.Chat{ID: "c1"}, 2, "title_set", false)
	s.ApplyLedger(ctx, "c1", snapshot.LedgerFacts{}, "turn_stopped")
	s.ApplyRunner(ctx, "c1", "r1", "exited")
	s.Touch(ctx, "c1")

	assert.Len(t, rec.frames, deleted, "nothing is published for a deleted chat")
	assert.Zero(t, s.Len(), "a deleted chat holds no entry")
	_, err := s.Get(ctx, "c1")
	require.Error(t, err)
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
// so a client holding an old version accepts the new daemon's answers. The
// previous daemon booted a millisecond ago and issued fewer versions than that.
func TestSnapshots_VersionsSurviveARestart(t *testing.T) {
	ctx := context.Background()
	rd := &reader{chats: map[string]domain.Chat{"c1": {ID: "c1"}}}
	before := snapshot.New(time.Now().Add(-time.Millisecond).UnixMicro())
	before.Bind(rd, newRuntime())
	for range 100 {
		before.Touch(ctx, "c1")
	}
	old, err := before.Get(ctx, "c1")
	require.NoError(t, err)

	after := snapshot.NewAtBoot()
	after.Bind(rd, newRuntime())
	fresh, err := after.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Greater(t, fresh.Version, old.Version)
}

// A read of many chats overlays the Node placement ONCE for the lot, and does it
// outside the owner's lock: the overlay reads the store, and a store read under
// the lock stalls every event and every other read behind it.
func TestSnapshots_GetManyCorrectsOncePerReadOutsideTheLock(t *testing.T) {
	rd := &reader{chats: map[string]domain.Chat{
		"a": {ID: "a", WorkspaceID: "w"}, "b": {ID: "b", WorkspaceID: "w"}, "c": {ID: "c", WorkspaceID: "w"},
	}}
	s, _, _ := newOwner(t, rd)
	var calls, batch int
	s.SetCorrect(func(_ context.Context, chats []domain.Chat) []domain.Chat {
		calls++
		batch = len(chats)
		_ = s.Len() // takes the owner's lock: deadlocks if the overlay runs under it
		out := append([]domain.Chat(nil), chats...)
		for i := range out {
			out[i].Order = 7
		}
		return out
	})

	got, err := s.GetMany(context.Background(), []string{"a", "b", "c"})

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, 1, calls)
	assert.Equal(t, 3, batch)
	for i, id := range []string{"a", "b", "c"} {
		assert.Equal(t, id, got[i].Chat.ID, "answers keep the order asked")
		assert.Equal(t, 7, got[i].Chat.Order)
	}
}

// The Tree announces a placement before its readers can all see it, so a chat
// loaded for that announcement keeps the announced placement, not the row read.
func TestSnapshots_AnAnnouncedPlacementWinsOverTheRowALoadReads(t *testing.T) {
	ctx := context.Background()
	s, rec, _ := newOwner(t, &reader{chats: map[string]domain.Chat{"c1": {ID: "c1", Title: "t"}}})

	s.ApplyPlacement(ctx, "c1", "f-1", 3, "placement_set")

	frame := rec.last(t)
	assert.Equal(t, "placement_set", frame.Kind)
	assert.Equal(t, "f-1", frame.Snapshot.Chat.ParentID)
	assert.Equal(t, 3, frame.Snapshot.Chat.Order)
	held, err := s.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, 3, held.Chat.Order, "the owner holds it for every later read")
	assert.Equal(t, "t", held.Chat.Title)
}

// A chat event read its placement when it was raised; one delivered after a
// later move keeps the move, because placement is the Tree's and announced apart.
func TestSnapshots_AChatEventNeverRedrawsAChatAtAPlaceItLeft(t *testing.T) {
	ctx := context.Background()
	s, rec, _ := newOwner(t, &reader{chats: map[string]domain.Chat{"c1": {ID: "c1", ParentID: "f-old"}}})
	s.ApplyChat(ctx, domain.Chat{ID: "c1", ParentID: "f-old"}, 1, "created", false)
	s.ApplyPlacement(ctx, "c1", "f-new", 2, "placement_set")

	s.ApplyChat(ctx, domain.Chat{ID: "c1", Title: "t", ParentID: "f-old"}, 2, "title_set", false)

	got := rec.last(t).Snapshot.Chat
	assert.Equal(t, "t", got.Title)
	assert.Equal(t, "f-new", got.ParentID)
	assert.Equal(t, 2, got.Order)
}

// A cold read of many chats loads every one it does not hold in one read, not
// one per chat: each placement read decodes a whole project's tree.
func TestSnapshots_GetManyLoadsTheChatsItLacksInOneRead(t *testing.T) {
	rd := &reader{chats: map[string]domain.Chat{
		"a": {ID: "a", Title: "A"}, "b": {ID: "b", Title: "B"}, "c": {ID: "c", Title: "C"},
	}}
	s, _, _ := newOwner(t, rd)
	ctx := context.Background()
	_, err := s.Get(ctx, "a")
	require.NoError(t, err)
	rd.gets, rd.batches = 0, 0

	got, err := s.GetMany(ctx, []string{"a", "b", "c"})

	require.NoError(t, err)
	assert.Equal(t, []string{"A", "B", "C"}, []string{got[0].Chat.Title, got[1].Chat.Title, got[2].Chat.Title})
	assert.Equal(t, 1, rd.batches)
	assert.Zero(t, rd.gets)
	_, err = s.GetMany(ctx, []string{"a", "b"})
	require.NoError(t, err)
	assert.Equal(t, 1, rd.batches, "a read of held chats reads nothing")
}

// A chat the batch does not find is reported as the single read reports it.
func TestSnapshots_GetManyOfAnUnknownChatFails(t *testing.T) {
	s, _, _ := newOwner(t, &reader{chats: map[string]domain.Chat{"a": {ID: "a"}}})

	_, err := s.GetMany(context.Background(), []string{"a", "nope"})

	require.Error(t, err)
	assert.Equal(t, 1, s.Len(), "the unknown chat holds no entry")
}

// gatedReader is reader whose batch read waits for release, after saying so on
// entered, and fails when it is asked for bad.
type gatedReader struct {
	*reader
	entered, release chan struct{}
	bad              string
}

func (r *gatedReader) GetChats(ctx context.Context, ids []string) ([]domain.Chat, error) {
	r.entered <- struct{}{}
	<-r.release
	for _, id := range ids {
		if id == r.bad {
			return nil, errors.New("corrupt history")
		}
	}
	return r.reader.GetChats(ctx, ids)
}

func newGated(t *testing.T, bad string) (*snapshot.Snapshots, *gatedReader) {
	t.Helper()
	rd := &reader{chats: map[string]domain.Chat{
		"a": {ID: "a", Title: "old"}, "x": {ID: "x", Title: "old", ParentID: "f-old"},
	}}
	gr := &gatedReader{reader: rd, entered: make(chan struct{}, 1), release: make(chan struct{}), bad: bad}
	s, _, _ := newOwner(t, rd)
	s.Bind(gr, newRuntime())
	return s, gr
}

// The batch read runs outside the owner's lock, and an event that lands while
// it runs wins over the row the batch read.
func TestSnapshots_AnEventDuringTheBatchReadWins(t *testing.T) {
	ctx := context.Background()
	cases := map[string]struct {
		event func(s *snapshot.Snapshots)
		check func(t *testing.T, s *snapshot.Snapshots, snaps []snapshot.Snapshot, err error)
	}{
		"a newer chat event": {
			event: func(s *snapshot.Snapshots) {
				s.ApplyChat(ctx, domain.Chat{ID: "x", Title: "new", ParentID: "f-old"}, 5, "title_set", false)
			},
			check: func(t *testing.T, _ *snapshot.Snapshots, snaps []snapshot.Snapshot, err error) {
				t.Helper()
				require.NoError(t, err)
				assert.Equal(t, "old", snaps[0].Chat.Title)
				assert.Equal(t, "new", snaps[1].Chat.Title)
			},
		},
		"a delete": {
			event: func(s *snapshot.Snapshots) { s.ApplyChat(ctx, domain.Chat{ID: "x"}, 5, "deleted", true) },
			check: func(t *testing.T, s *snapshot.Snapshots, _ []snapshot.Snapshot, err error) {
				t.Helper()
				require.ErrorIs(t, err, snapshot.ErrDeleted)
				assert.Equal(t, 1, s.Len(), "the batch holds no entry for the deleted chat")
				_, err = s.Get(ctx, "x")
				require.ErrorIs(t, err, snapshot.ErrDeleted, "the batch does not bring the chat back")
			},
		},
		"a move": {
			event: func(s *snapshot.Snapshots) { s.ApplyPlacement(ctx, "x", "f-new", 3, "placement_set") },
			check: func(t *testing.T, _ *snapshot.Snapshots, snaps []snapshot.Snapshot, err error) {
				t.Helper()
				require.NoError(t, err)
				assert.Equal(t, "f-new", snaps[1].Chat.ParentID)
				assert.Equal(t, 3, snaps[1].Chat.Order)
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s, gr := newGated(t, "")
			type result struct {
				snaps []snapshot.Snapshot
				err   error
			}
			read := make(chan result, 1)
			go func() {
				snaps, err := s.GetMany(ctx, []string{"a", "x"})
				read <- result{snaps, err}
			}()
			<-gr.entered
			applied := make(chan struct{})
			go func() { tc.event(s); close(applied) }()
			require.Eventually(t, func() bool {
				select {
				case <-applied:
					return true
				default:
					return false
				}
			}, 5*time.Second, time.Millisecond, "an event is applied while the batch read runs")

			close(gr.release)
			got := <-read
			tc.check(t, s, got.snaps, got.err)
		})
	}
}

// A chat the batch cannot read fails the whole read, as the one-chat read did.
func TestSnapshots_AChatTheBatchCannotReadFailsTheRead(t *testing.T) {
	s, gr := newGated(t, "x")
	close(gr.release)
	go func() { <-gr.entered }()

	_, err := s.GetMany(context.Background(), []string{"a", "x"})

	require.Error(t, err)
	got, err := s.GetMany(context.Background(), []string{"a"})
	require.NoError(t, err, "the readable chat still answers on its own")
	assert.Equal(t, "old", got[0].Chat.Title)
}
