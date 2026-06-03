package hub

import (
	"sync"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/hub"
)

// stubSubscriber is a minimal Subscriber used in tests only.
// Each Push* method appends to its own slice; channels are used for
// race-safe concurrent collection.
type stubSubscriber struct {
	mu           sync.Mutex
	tasks        []domain.Task
	agentRuns    []domain.AgentRun
	kanbanItems  []domain.KanbanItem
	reviewThreads []domain.ReviewThread
}

func (s *stubSubscriber) PushTask(t domain.Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = append(s.tasks, t)
}

func (s *stubSubscriber) PushAgentRun(r domain.AgentRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentRuns = append(s.agentRuns, r)
}

func (s *stubSubscriber) PushKanbanItem(i domain.KanbanItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kanbanItems = append(s.kanbanItems, i)
}

func (s *stubSubscriber) PushReviewThread(t domain.ReviewThread) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviewThreads = append(s.reviewThreads, t)
}

// ---------------------------------------------------------------------------
// NewHub
// ---------------------------------------------------------------------------

func TestNewHub_returnsNonNil(t *testing.T) {
	h := NewHub()
	assert.NotNil(t, h)
}

// ---------------------------------------------------------------------------
// Register + Broadcast fan-out
// ---------------------------------------------------------------------------

func TestBroadcastTask_fansOutToAllSubscribers(t *testing.T) {
	h := NewHub()
	s1 := &stubSubscriber{}
	s2 := &stubSubscriber{}

	h.Register(s1)
	h.Register(s2)

	task := domain.Task{ID: "t1", Title: "first task"}
	h.BroadcastTask(task)

	assert.Equal(t, []domain.Task{task}, s1.tasks)
	assert.Equal(t, []domain.Task{task}, s2.tasks)
}

func TestBroadcastAgentRun_fansOutToAllSubscribers(t *testing.T) {
	h := NewHub()
	s1 := &stubSubscriber{}
	s2 := &stubSubscriber{}

	h.Register(s1)
	h.Register(s2)

	run := domain.AgentRun{ID: "r1", TaskID: "t1"}
	h.BroadcastAgentRun(run)

	assert.Equal(t, []domain.AgentRun{run}, s1.agentRuns)
	assert.Equal(t, []domain.AgentRun{run}, s2.agentRuns)
}

func TestBroadcastKanbanItem_fansOutToAllSubscribers(t *testing.T) {
	h := NewHub()
	s1 := &stubSubscriber{}
	s2 := &stubSubscriber{}

	h.Register(s1)
	h.Register(s2)

	item := domain.KanbanItem{ID: "k1", TaskID: "t1", Title: "do thing"}
	h.BroadcastKanbanItem(item)

	assert.Equal(t, []domain.KanbanItem{item}, s1.kanbanItems)
	assert.Equal(t, []domain.KanbanItem{item}, s2.kanbanItems)
}

func TestBroadcastReviewThread_fansOutToAllSubscribers(t *testing.T) {
	h := NewHub()
	s1 := &stubSubscriber{}
	s2 := &stubSubscriber{}

	h.Register(s1)
	h.Register(s2)

	thread := domain.ReviewThread{ID: "th1", TaskID: "t1", File: "main.go", Line: 42}
	h.BroadcastReviewThread(thread)

	assert.Equal(t, []domain.ReviewThread{thread}, s1.reviewThreads)
	assert.Equal(t, []domain.ReviewThread{thread}, s2.reviewThreads)
}

// ---------------------------------------------------------------------------
// No subscribers
// ---------------------------------------------------------------------------

func TestBroadcastTask_noSubscribers_noOp(t *testing.T) {
	h := NewHub()
	// Must not panic.
	h.BroadcastTask(domain.Task{ID: "t1"})
}

func TestBroadcastAgentRun_noSubscribers_noOp(t *testing.T) {
	h := NewHub()
	h.BroadcastAgentRun(domain.AgentRun{ID: "r1"})
}

func TestBroadcastKanbanItem_noSubscribers_noOp(t *testing.T) {
	h := NewHub()
	h.BroadcastKanbanItem(domain.KanbanItem{ID: "k1"})
}

func TestBroadcastReviewThread_noSubscribers_noOp(t *testing.T) {
	h := NewHub()
	h.BroadcastReviewThread(domain.ReviewThread{ID: "th1"})
}

// ---------------------------------------------------------------------------
// Unregister stops delivery
// ---------------------------------------------------------------------------

func TestUnregister_stopsDelivery(t *testing.T) {
	h := NewHub()
	s1 := &stubSubscriber{}
	s2 := &stubSubscriber{}

	h.Register(s1)
	h.Register(s2)

	h.Unregister(s1)

	task := domain.Task{ID: "t1"}
	h.BroadcastTask(task)

	assert.Empty(t, s1.tasks, "unregistered subscriber must not receive broadcasts")
	assert.Equal(t, []domain.Task{task}, s2.tasks)
}

func TestUnregister_onlySubscriber_noOp(t *testing.T) {
	h := NewHub()
	s := &stubSubscriber{}

	h.Register(s)
	h.Unregister(s)

	h.BroadcastTask(domain.Task{ID: "t1"})
	assert.Empty(t, s.tasks)
}

func TestUnregister_unknownSubscriber_noOp(t *testing.T) {
	h := NewHub()
	s1 := &stubSubscriber{}
	s2 := &stubSubscriber{} // never registered

	h.Register(s1)
	h.Unregister(s2) // must not remove s1

	task := domain.Task{ID: "t1"}
	h.BroadcastTask(task)

	assert.Equal(t, []domain.Task{task}, s1.tasks)
}

// ---------------------------------------------------------------------------
// Multiple broadcasts accumulate
// ---------------------------------------------------------------------------

func TestBroadcastTask_multipleBroadcasts_accumulate(t *testing.T) {
	h := NewHub()
	s := &stubSubscriber{}
	h.Register(s)

	t1 := domain.Task{ID: "t1"}
	t2 := domain.Task{ID: "t2"}
	h.BroadcastTask(t1)
	h.BroadcastTask(t2)

	assert.Equal(t, []domain.Task{t1, t2}, s.tasks)
}

// ---------------------------------------------------------------------------
// Concurrent broadcasts do not race (run with -race)
// ---------------------------------------------------------------------------

func TestBroadcastTask_concurrent_noRace(t *testing.T) {
	h := NewHub()

	const numSubs = 5
	subs := make([]*stubSubscriber, numSubs)
	for i := range subs {
		subs[i] = &stubSubscriber{}
		h.Register(subs[i])
	}

	var wg sync.WaitGroup
	const broadcasts = 50
	wg.Add(broadcasts)
	for i := 0; i < broadcasts; i++ {
		go func(n int) {
			defer wg.Done()
			h.BroadcastTask(domain.Task{ID: "t"})
		}(i)
	}
	wg.Wait()

	for _, s := range subs {
		s.mu.Lock()
		assert.Len(t, s.tasks, broadcasts)
		s.mu.Unlock()
	}
}

func TestRegisterAndUnregister_concurrent_noRace(t *testing.T) {
	h := NewHub()

	var wg sync.WaitGroup
	const n = 20

	// Concurrent registers.
	subs := make([]*stubSubscriber, n)
	for i := range subs {
		subs[i] = &stubSubscriber{}
	}

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			h.Register(subs[idx])
		}(i)
	}
	wg.Wait()

	// Concurrent unregisters interleaved with broadcasts.
	wg.Add(n + 1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			h.BroadcastTask(domain.Task{ID: "t"})
		}
	}()
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			h.Unregister(subs[idx])
		}(i)
	}
	wg.Wait()
}

func TestBroadcastAllTypes_concurrent_noRace(t *testing.T) {
	h := NewHub()
	s := &stubSubscriber{}
	h.Register(s)

	var wg sync.WaitGroup
	wg.Add(4)
	go func() { defer wg.Done(); h.BroadcastTask(domain.Task{ID: "t1"}) }()
	go func() { defer wg.Done(); h.BroadcastAgentRun(domain.AgentRun{ID: "r1"}) }()
	go func() { defer wg.Done(); h.BroadcastKanbanItem(domain.KanbanItem{ID: "k1"}) }()
	go func() { defer wg.Done(); h.BroadcastReviewThread(domain.ReviewThread{ID: "th1"}) }()
	wg.Wait()
}

// ---------------------------------------------------------------------------
// WebSocketHub interface satisfaction
// ---------------------------------------------------------------------------

func TestHub_implementsWebSocketHub(t *testing.T) {
	var _ WebSocketHub = (*Hub)(nil)
}
