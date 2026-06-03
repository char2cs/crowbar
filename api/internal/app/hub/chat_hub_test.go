package hub_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/engine/agent"
)

func TestNewChatHub_returnsNonNil(t *testing.T) {
	h := hub.NewChatHub()
	assert.NotNil(t, h)
}

func TestRegisterSession_createsInputChannelAndPublishFunc(t *testing.T) {
	h := hub.NewChatHub()
	in, pub, unreg := h.RegisterSession("task-1")

	require.NotNil(t, in)
	require.NotNil(t, pub)
	require.NotNil(t, unreg)
}

func TestSubscribe_returnsFramesChannelAndUnsubscribeFunc(t *testing.T) {
	h := hub.NewChatHub()
	frames, unsub := h.Subscribe("task-1")

	require.NotNil(t, frames)
	require.NotNil(t, unsub)
}

func TestForward_withSession_deliversToInputChannel(t *testing.T) {
	h := hub.NewChatHub()
	in, _, _ := h.RegisterSession("task-1")

	h.Forward("task-1", "hello")

	select {
	case got := <-in:
		assert.Equal(t, "hello", got)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Forward: message never arrived on input channel")
	}
}

func TestForward_noSession_silentlyDropped(t *testing.T) {
	h := hub.NewChatHub()
	h.Forward("task-x", "ignored")
}

func TestForward_fullChannel_doesNotBlock(t *testing.T) {
	h := hub.NewChatHub()
	in, _, _ := h.RegisterSession("task-full")

	for i := 0; i < 32; i++ {
		h.Forward("task-full", "msg")
	}
	done := make(chan struct{})
	go func() {
		h.Forward("task-full", "overflow")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Forward with full channel blocked")
	}
	_ = in
}

func TestPublish_fansOutToSubscribersForTaskID(t *testing.T) {
	h := hub.NewChatHub()

	frames1, unsub1 := h.Subscribe("task-1")
	frames2, unsub2 := h.Subscribe("task-1")
	defer unsub1()
	defer unsub2()

	frame := agent.ChatFrame{Type: agent.ChatFrameTypeAgentChunk, Delta: "hi"}
	h.Publish("task-1", frame)

	recv := func(ch <-chan agent.ChatFrame) agent.ChatFrame {
		select {
		case f := <-ch:
			return f
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Publish: no frame received")
			return agent.ChatFrame{}
		}
	}

	assert.Equal(t, frame, recv(frames1))
	assert.Equal(t, frame, recv(frames2))
}

func TestPublish_doesNotDeliverToOtherTaskID(t *testing.T) {
	h := hub.NewChatHub()

	_, unsub1 := h.Subscribe("task-1")
	frames2, unsub2 := h.Subscribe("task-2")
	defer unsub1()
	defer unsub2()

	h.Publish("task-1", agent.ChatFrame{Type: agent.ChatFrameTypeAgentChunk, Delta: "x"})

	select {
	case <-frames2:
		t.Fatal("frame delivered to wrong subscriber")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPublish_noSubscribers_noOp(t *testing.T) {
	h := hub.NewChatHub()
	h.Publish("task-none", agent.ChatFrame{Type: agent.ChatFrameTypeAgentChunk})
}

func TestPublish_fullChannel_doesNotBlock(t *testing.T) {
	h := hub.NewChatHub()
	frames, unsub := h.Subscribe("task-full")
	defer unsub()

	frame := agent.ChatFrame{Type: agent.ChatFrameTypeAgentChunk, Delta: "x"}

	for i := 0; i < 64; i++ {
		h.Publish("task-full", frame)
	}

	done := make(chan struct{})
	go func() {
		h.Publish("task-full", frame)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Publish with full channel blocked")
	}
	_ = frames
}

func TestUnregister_forwardNoLongerDelivers(t *testing.T) {
	h := hub.NewChatHub()
	in, _, unreg := h.RegisterSession("task-1")

	unreg()
	h.Forward("task-1", "gone")

	select {
	case <-in:
		t.Fatal("message delivered after unregister")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestUnregister_publishNoLongerDelivers(t *testing.T) {
	h := hub.NewChatHub()
	_, pub, unreg := h.RegisterSession("task-1")

	frames, unsub := h.Subscribe("task-1")
	defer unsub()

	unreg()
	pub(agent.ChatFrame{Type: agent.ChatFrameTypeAgentChunk, Delta: "after-unreg"})

	select {
	case f := <-frames:
		assert.Equal(t, "after-unreg", f.Delta)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("publish func should still deliver to subscribers after unregister")
	}
}

func TestUnregister_calledTwice_noOp(t *testing.T) {
	h := hub.NewChatHub()
	_, _, unreg := h.RegisterSession("task-1")
	unreg()
	unreg()
}

func TestUnsubscribe_publishNoLongerDelivers(t *testing.T) {
	h := hub.NewChatHub()
	frames, unsub := h.Subscribe("task-1")

	unsub()

	h.Publish("task-1", agent.ChatFrame{Type: agent.ChatFrameTypeAgentChunk, Delta: "gone"})

	select {
	case <-frames:
		t.Fatal("frame delivered after unsubscribe")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestUnsubscribe_onlyRemovesSelf(t *testing.T) {
	h := hub.NewChatHub()

	frames1, unsub1 := h.Subscribe("task-1")
	frames2, unsub2 := h.Subscribe("task-1")
	defer unsub2()

	unsub1()

	frame := agent.ChatFrame{Type: agent.ChatFrameTypeAgentChunk, Delta: "still here"}
	h.Publish("task-1", frame)

	select {
	case got := <-frames2:
		assert.Equal(t, frame, got)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("remaining subscriber did not receive frame after other unsubscribed")
	}
	select {
	case <-frames1:
		t.Fatal("unsubscribed channel still received")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestUnsubscribe_calledTwice_noOp(t *testing.T) {
	h := hub.NewChatHub()
	_, unsub := h.Subscribe("task-1")
	unsub()
	unsub()
}
