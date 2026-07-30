package agenttools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatLogReader is the narrow read port get_chat_log needs from the agent
// usecase: the rendered ledger for one chat. It is consulted only AFTER the
// tool has confirmed the chat's workspace is in the caller's visible set — a
// chat id is not itself an authorization, so nothing here does its own scope
// check.
type ChatLogReader interface {
	ReadChatLog(ctx context.Context, chatID string) (string, error)
}

// NoChatTurnsText is what get_chat_log renders for a chat that has not spoken
// yet — a NORMAL state, not an error and not empty text. A model handed an
// error would try to work around a failure that is not one; handed an empty
// result, it would likely retry or report a broken tool. Exported so
// agent.Usecase's ReadChatLog (the production ChatLogReader) renders the exact
// same wording rather than the two ever drifting apart.
const NoChatTurnsText = "This chat has no turns yet."

// contextTools registers the tools that give an agent situational awareness of
// the workspaces and sibling chats around it, each independently fail-closed on
// its own dependency: a nil port means the tool that needs it is simply not
// advertised.
//
// list_workspaces needs only ChatReads: it folds each visible workspace's chats
// in via ListByWorkspace rather than getting its own tool, to stay under the
// tool ceiling. get_chat_log needs BOTH ChatReads (to resolve a chat id to the
// workspace it belongs to, before anything is read from disk) and ChatLogs (the
// actual ledger read) — with either missing, reading another chat's log is not a
// capability that safely exists.
func contextTools(deps Deps) []toolDef {
	var out []toolDef
	if deps.ChatReads != nil {
		out = append(out, listWorkspacesTool(deps))
	}
	if canReadChatLog(deps) {
		out = append(out, getChatLogTool(deps))
	}
	return out
}

func canReadChatLog(deps Deps) bool {
	return deps.ChatReads != nil && deps.ChatLogs != nil
}

func listWorkspacesTool(deps Deps) toolDef {
	return toolDef{
		name:        "list_workspaces",
		description: "List the workspaces this chat can see — itself and its children, or the whole repo or project depending on where it runs — each with its chats.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{},
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, _ json.RawMessage) (string, error) {
			return listWorkspaces(ctx, deps, c)
		},
	}
}

// listWorkspaces folds every visible workspace's chats in here rather than
// giving chats their own tool, to stay under the ceiling. c.Visible is the
// resolver's own downward-only computation — there is no argument here a model
// could widen it with.
//
// The chats arrive in ONE read and are bucketed in Go. Asking the store per
// visible workspace looked cheap and was not: ListByWorkspace is itself a full
// scan of agent_chats_read plus a json.Unmarshal of every row, filtered in Go
// afterwards, so a caller seeing V workspaces read and decoded the entire chat
// table V times to render it once.
func listWorkspaces(
	ctx context.Context,
	deps Deps,
	c Caller,
) (string, error) {
	visible, err := c.Visible()
	if err != nil {
		return "", fmt.Errorf("agenttools: list_workspaces: %w", err)
	}
	all, err := deps.ChatReads.ListChats(ctx)
	if err != nil {
		return "", fmt.Errorf("agenttools: list_workspaces: %w", err)
	}
	return renderWorkspaces(c.Workspace, visible, chatsByWorkspace(all, visible)), nil
}

// chatsByWorkspace buckets one whole-table chat read by owning workspace,
// keeping ONLY the workspaces in visible.
//
// The visible ids are seeded first and the membership test is what filters:
// every workspace the caller can see gets an entry (so one with no chats still
// renders as itself rather than vanishing), and a chat whose workspace is not
// among them is dropped rather than bucketed. That filter is the same authority
// boundary the per-workspace query used to enforce implicitly by never asking
// about anything else — moving the read out of the loop must not move the
// boundary with it.
func chatsByWorkspace(
	all []domain.AgentChat,
	visible []domain.Workspace,
) map[string][]domain.AgentChat {
	out := make(map[string][]domain.AgentChat, len(visible))
	for _, w := range visible {
		out[w.ID] = nil
	}
	for _, chat := range all {
		if _, ok := out[chat.WorkspaceID]; !ok {
			continue
		}
		out[chat.WorkspaceID] = append(out[chat.WorkspaceID], chat)
	}
	return out
}

func getChatLogTool(deps Deps) toolDef {
	return toolDef{
		name:        "get_chat_log",
		description: "Read the conversation of another chat you can see. Get chat ids from list_workspaces.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"chatId":{"type":"string","description":"The chat to read. Get chat ids from list_workspaces."}
			},
			"required":["chatId"],
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			return getChatLog(ctx, deps, c, args)
		},
	}
}

type getChatLogArgs struct {
	ChatID string `json:"chatId"`
}

// getChatLog is the whole point of the scope model made concrete: a chat id
// names a chat in SOME workspace, and that is not itself a reason to let the
// caller read it. The chat is resolved to its owning workspace FIRST, that
// workspace is checked against c.Visible, and only once it passes does
// ChatLogs.ReadChatLog — the thing that actually touches disk — get called.
// A rejection here must never reach ChatLogs at all.
func getChatLog(
	ctx context.Context,
	deps Deps,
	c Caller,
	args json.RawMessage,
) (string, error) {
	var in getChatLogArgs
	if err := decode(args, &in); err != nil {
		return "", err
	}
	chat, err := deps.ChatReads.Get(ctx, in.ChatID)
	if err != nil {
		return "", fmt.Errorf("agenttools: get_chat_log: %w", err)
	}
	if !c.CanSee(chat.WorkspaceID) {
		return "", fmt.Errorf("agenttools: get_chat_log: %w", ErrOutOfScope)
	}
	out, err := deps.ChatLogs.ReadChatLog(ctx, in.ChatID)
	if err != nil {
		return "", fmt.Errorf("agenttools: get_chat_log: %w", err)
	}
	// Rendering the empty case is THIS function's job, and only this one's. A
	// chat with nothing recorded yet comes back as "" — the production reader
	// (agent.Usecase.ReadChatLog) deliberately returns the empty string rather
	// than wording of its own, so the phrasing lives in exactly one place instead
	// of drifting between two. Any other ChatLogReader gets the same treatment.
	if out == "" {
		out = NoChatTurnsText
	}
	return out, nil
}
