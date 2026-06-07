package v0

// ChatFrame is the post-spike ChatStream payload, scoped per chat by ChatID
// (03 §3, §4.3). The ChatStream broadcaster and its
// GET /v0/ws/chats/:chatId/stream route are registered now so the topology is
// complete, but NOTHING pushes frames to it yet — the frame protocol is deferred
// to the agentic bridge spike (03 §8, 12). This is a placeholder shape only.
type ChatFrame struct {
	ChatID string `json:"chatId"`
}
