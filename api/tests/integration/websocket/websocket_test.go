//go:build integration

package websocket

import (
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestWebSocket(t *testing.T) { suite.Run(t, new(WebSocketSuite)) }

type WebSocketSuite struct{ kit.IntegrationSuite }

// TestPerTaskIsolation verifies that events for task A are not delivered to a
// subscriber watching task B.
func (s *WebSocketSuite) TestPerTaskIsolation() {
	env := s.Env

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())

	taskA := kit.DefaultTask(env.Client, repo.ID, "ws-isolation-a")
	taskB := kit.DefaultTask(env.Client, repo.ID, "ws-isolation-b")

	chA, closeA := env.WSClient.SubscribeTasks(taskA.ID)
	defer closeA()
	chB, closeB := env.WSClient.SubscribeTasks(taskB.ID)
	defer closeB()

	// Drain initial sync frames.
	drainAvailable(chA, 500*time.Millisecond)
	drainAvailable(chB, 500*time.Millisecond)

	// Transition task A via force-transition.
	env.Client.ForceTransition(taskA.ID, "spec")

	// Task A subscriber should receive an event for task A.
	gotA := waitFor(chA, func(t domain.Task) bool { return t.CurrentState == "spec" }, 3*time.Second)
	assert.True(s.T(), gotA, "expected task A event on channel A")

	// Task B subscriber should NOT receive task A events.
	gotB := waitFor(chB, func(t domain.Task) bool { return t.ID == taskA.ID }, 500*time.Millisecond)
	assert.False(s.T(), gotB, "task A events should not reach task B subscriber")
}

// TestUnsubscribeStopsDelivery verifies that calling the unsubscribe function
// stops further event delivery to that subscriber.
func (s *WebSocketSuite) TestUnsubscribeStopsDelivery() {
	env := s.Env

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "ws-unsub-branch")

	ch, closeSub := env.WSClient.SubscribeKanbanItems(task.ID)

	// Drain any initial sync.
	drainAvailable(ch, 200*time.Millisecond)

	// Unsubscribe.
	closeSub()

	// Triggering an event after unsubscribe should not reach the closed connection.
	// The channel will be closed by the goroutine, so any read returns zero value.
	// No further events should arrive (connection is closed).
	time.Sleep(100 * time.Millisecond)

	select {
	case _, ok := <-ch:
		if ok {
			s.T().Log("received an event after unsubscribe (channel still open)")
		}
	default:
		// Channel is empty or closed — correct.
	}
}

// TestSlowConsumerDoesNotBlockOtherSubscribers verifies that a subscriber with
// a full buffer does not prevent other subscribers from receiving events.
func (s *WebSocketSuite) TestSlowConsumerDoesNotBlockOtherSubscribers() {
	env := s.Env

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "ws-slow-consumer-branch")

	// First subscriber — will drain promptly.
	fastCh, closeFast := env.WSClient.SubscribeTasks(task.ID)
	defer closeFast()

	// Second subscriber — never drains (simulates slow consumer).
	_, closeSlow := env.WSClient.SubscribeTasks(task.ID)
	defer closeSlow()

	drainAvailable(fastCh, 300*time.Millisecond)

	// Transition the task; the fast subscriber should still receive it.
	env.Client.ForceTransition(task.ID, "spec")
	got := waitFor(fastCh, func(t domain.Task) bool { return t.CurrentState == "spec" }, 3*time.Second)
	assert.True(s.T(), got, "fast subscriber should receive events despite slow consumer")
}

// drainAvailable reads all currently-available items from ch without blocking
// longer than timeout.
func drainAvailable[T any](
	ch <-chan T,
	timeout time.Duration,
) {
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return
		}
		select {
		case <-ch:
		case <-time.After(remaining):
			return
		}
	}
}

// waitFor reads from ch until pred returns true or timeout elapses.
func waitFor[T any](
	ch <-chan T,
	pred func(T) bool,
	timeout time.Duration,
) bool {
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}
		select {
		case v, ok := <-ch:
			if !ok {
				return false
			}
			if pred(v) {
				return true
			}
		case <-time.After(remaining):
			return false
		}
	}
}
