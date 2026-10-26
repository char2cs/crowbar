package v0

import (
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

// chatPresence counts the attach sockets open on each chat. A socket's lifetime
// IS the attachment, so a closed pane, a dropped phone and a revoked client all
// detach through the same exit, and the agent never hears of any of them.
type chatPresence struct {
	mu    sync.Mutex
	chats map[string]*chatViewers
}

type chatViewers struct {
	workspaceID string
	count       int
}

func newChatPresence() *chatPresence {
	return &chatPresence{chats: map[string]*chatViewers{}}
}

// joinChat counts one more viewer of chatID and returns the leave that undoes it.
// The change is announced under the lock so the last frame on the feed is
// always the current count.
func (c *Container) joinChat(
	chatID string,
	workspaceID string,
) func() {
	scope := c.agentChatScope(chatID, workspaceID)
	p := c.presence
	p.mu.Lock()
	defer p.mu.Unlock()
	v := p.chats[chatID]
	if v == nil {
		v = &chatViewers{workspaceID: workspaceID}
		p.chats[chatID] = v
	}
	v.count++
	c.agentChats.Push(attachedEvent(chatID, workspaceID, scope, v.count))
	var once sync.Once
	return func() { once.Do(func() { c.leaveChat(chatID, workspaceID, scope) }) }
}

func (c *Container) leaveChat(
	chatID string,
	workspaceID string,
	scope chatScope,
) {
	p := c.presence
	p.mu.Lock()
	defer p.mu.Unlock()
	v := p.chats[chatID]
	v.count--
	if v.count == 0 {
		delete(p.chats, chatID)
	}
	c.agentChats.Push(attachedEvent(chatID, workspaceID, scope, v.count))
}

// attachedSnapshot is one frame per chat with viewers, for a socket that connects
// after they attached.
func (c *Container) attachedSnapshot() []dto.AgentChatEvent {
	p := c.presence
	p.mu.Lock()
	held := make(map[string]chatViewers, len(p.chats))
	for id, v := range p.chats {
		held[id] = *v
	}
	p.mu.Unlock()
	out := make([]dto.AgentChatEvent, 0, len(held))
	for id, v := range held {
		out = append(out, attachedEvent(id, v.workspaceID, c.agentChatScope(id, v.workspaceID), v.count))
	}
	return out
}

func attachedEvent(
	chatID string,
	workspaceID string,
	scope chatScope,
	count int,
) dto.AgentChatEvent {
	return dto.AgentChatEvent{
		ChatID:      chatID,
		WorkspaceID: workspaceID,
		ProjectID:   scope.ProjectID,
		RepoID:      scope.RepoID,
		Kind:        dto.AgentChatKindAttached,
		Attached:    &count,
	}
}

// attachChat serves GET /v0/chats/:chatId/attach: the socket a client holds open
// for as long as it displays the chat.
func (c *Container) attachChat(
	g *gin.Context,
) {
	ws.Hold(g, func() func() {
		return c.joinChat(g.Param("chatId"), scopeChatWorktreeID(g))
	})
}
