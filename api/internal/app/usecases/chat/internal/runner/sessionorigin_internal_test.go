package runner

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mintingServer answers every request on this connection with one thread id —
// enough for the prompt event's fresh thread/start step (carrierTestDescriptor)
// and nothing more, since the establish is the only call under test.
func mintingServer(t *testing.T, threadID string) string {
	t.Helper()
	return fakeWSServer(t, func(conn *websocket.Conn) {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return // the client closed; the test's own assertions report it
			}
			var req struct {
				ID json.RawMessage `json:"id"`
			}
			if json.Unmarshal(msg, &req) != nil || len(req.ID) == 0 {
				continue // a notification needs no reply
			}
			resp, _ := json.Marshal(map[string]any{
				"id":     req.ID,
				"result": map[string]any{"thread": map[string]string{"id": threadID}},
			})
			if conn.WriteMessage(websocket.TextMessage, resp) != nil {
				return
			}
		}
	})
}

// TestRegression_TheSessionEstablishedAtSpawnIsRecordedAsCrowbarsOwn pins the
// seam the 2026-09-23 collab-agents transcript bleed ran through, end to end
// over a REAL driver: only the driver's RECOVERY paths used to claim what they
// produced, so the thread a chat's own spawn minted was never recorded here.
// The hook ingress (turn.namesAnotherConversation) reads exactly this to tell
// the runner's own conversation from the child threads codex pushes down the
// same websocket — and a row that names no session, which is what a pure
// api-transport spawn leaves behind, gives it nothing else to go on.
func TestRegression_TheSessionEstablishedAtSpawnIsRecordedAsCrowbarsOwn(t *testing.T) {
	sockPath := mintingServer(t, "thread-minted")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	agent := installCarrierDescriptor(t, t.TempDir())
	// Built before the driver and handed to it, exactly as startAPIConn does.
	originated := newOriginatedSessions()
	driver, err := agent.StartAPIConn(ctx, sockPath, originated.Claim)
	require.NoError(t, err)
	defer driver.Close()

	rs := &Runners{apiConns: newAPIConnRegistry()}
	rs.apiConns.set("runner-1", &apiconn{driver: driver, ctx: ctx, originated: originated})

	established, err := driver.EstablishSession(ctx, "prompt", map[string]string{"cwd": "/work"})
	require.NoError(t, err)
	require.Equal(t, "thread-minted", established["session_id"])

	assert.True(t, rs.OriginatedSession("runner-1", established["session_id"]),
		"the conversation this chat's own establish minted must be recorded as Crowbar's, or every "+
			"child thread the provider opens on this connection reads as the user's own")
	assert.False(t, rs.OriginatedSession("runner-1", "thread-child"),
		"and a conversation nothing here opened must stay unclaimed — the claim is closed, not left pending")
	assert.False(t, rs.OriginatedSession("runner-2", "thread-minted"),
		"the record belongs to ONE connection; a runner without one originated nothing")
}
