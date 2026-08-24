package fanout_test

import (
	"testing"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/fanout"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

type chatFrame struct {
	chatID, workspaceID, kind string
	working                   bool
}

type runnerFrame struct {
	runnerID, workspaceID, chatID, kind string
}

type spyHub struct {
	chats   []chatFrame
	runners []runnerFrame
}

func (h *spyHub) BroadcastAgentChat(chatID, workspaceID, kind string, working bool) {
	h.chats = append(h.chats, chatFrame{chatID, workspaceID, kind, working})
}

func (h *spyHub) BroadcastAgentRunner(runnerID, workspaceID, chatID, kind string) {
	h.runners = append(h.runners, runnerFrame{runnerID, workspaceID, chatID, kind})
}

func TestFanout_ChatEvent_ReachesHubAsAFrame(t *testing.T) {
	hub := &spyHub{}
	f := fanout.New(hub)

	f.ChatWatch()(agentchat.ChatEvent{
		ChatID: "chat-1", WorkspaceID: "ws-1", Kind: "turn_started", Working: true,
	})

	if len(hub.chats) != 1 {
		t.Fatalf("want 1 frame, got %d", len(hub.chats))
	}
	want := chatFrame{"chat-1", "ws-1", "turn_started", true}
	if hub.chats[0] != want {
		t.Fatalf("got %+v, want %+v", hub.chats[0], want)
	}
}

// A forgotten chat is announced as deleted and NOT working — the client drops it.
// The repository still reports the aggregate's last-known Working, so suppressing it
// is the fanout's job and this is the test that pins it.
func TestFanout_ForgottenChat_IsNeverWorking(t *testing.T) {
	hub := &spyHub{}
	f := fanout.New(hub)

	f.ChatWatch()(agentchat.ChatEvent{
		ChatID: "chat-9", WorkspaceID: "ws-1", Kind: "deleted", Working: true, Forgotten: true,
	})

	if hub.chats[0].working {
		t.Fatal("a forgotten chat must never be announced as working")
	}
	if hub.chats[0].kind != "deleted" {
		t.Fatalf("kind = %q, want deleted", hub.chats[0].kind)
	}
}

func TestFanout_RunnerEvent_ReachesHub(t *testing.T) {
	hub := &spyHub{}
	f := fanout.New(hub)

	f.RunnerWatch()(agentrunner.RunnerEvent{
		RunnerID: "run-1", WorkspaceID: "ws-1", ChatID: "chat-1", Kind: "started",
	})

	if len(hub.runners) != 1 {
		t.Fatalf("want 1 frame, got %d", len(hub.runners))
	}
	want := runnerFrame{"run-1", "ws-1", "chat-1", "started"}
	if hub.runners[0] != want {
		t.Fatalf("got %+v, want %+v", hub.runners[0], want)
	}
}

// A displaced runner is pointed at no chat, and the empty id must survive the hop —
// it is what tells a client to stop showing that runner as the chat's agent.
func TestFanout_DisplacedRunner_KeepsAnEmptyChatID(t *testing.T) {
	hub := &spyHub{}
	f := fanout.New(hub)

	f.RunnerWatch()(agentrunner.RunnerEvent{
		RunnerID: "run-2", WorkspaceID: "ws-1", Kind: "displaced",
	})

	if hub.runners[0].chatID != "" {
		t.Fatalf("chatID = %q, want empty", hub.runners[0].chatID)
	}
}

// A daemon wired without a hub (every unit test) must not panic.
func TestFanout_NilHub_IsANoOp(t *testing.T) {
	f := fanout.New(nil)
	f.ChatWatch()(agentchat.ChatEvent{ChatID: "chat-1"})
	f.RunnerWatch()(agentrunner.RunnerEvent{RunnerID: "run-1"})
}

// The runner store REFUSES a nil watch, so RunnerWatch must always return a usable
// func even with no hub — returning nil here would make the container fail to build.
func TestFanout_RunnerWatch_IsNeverNil(t *testing.T) {
	if fanout.New(nil).RunnerWatch() == nil {
		t.Fatal("RunnerWatch must never be nil: runner.store.New refuses one")
	}
}
