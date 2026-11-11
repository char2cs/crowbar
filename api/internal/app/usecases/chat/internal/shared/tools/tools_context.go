package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools/internal/render"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatLogReader is get_chat_log's read port: one chat's conversation as turns, consulted only after the
// tool's scope checks. Turns, not rendered text, because bodies contain blank lines and the cap must
// count real turns.
type ChatLogReader interface {
	ReadChatLog(ctx context.Context, chatID string) ([]ChatTurn, error)
}

// ChatLineageReader resolves a chat's chat ancestors, nearest first, folders filtered out. get_chat_log
// asks it about the target, one upward walk, to tell a descendant from a sibling, which workspace scope
// cannot.
type ChatLineageReader interface {
	Ancestors(
		ctx context.Context,
		chatID string,
	) ([]string, error)
}

// NoChatTurnsText is what get_chat_log renders for a chat that has not spoken yet: a normal state, so
// neither an error nor empty text a model would retry.
const NoChatTurnsText = "This chat has no turns yet."

// contextTools registers the workspace and sibling-chat tools, each fail-closed on its own ports.
// get_chat_log needs ChatReads, ChatLogs and Lineage: without Lineage it could not refuse a parent
// reading its own threads, and an authority port that cannot answer takes its tool with it.
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
	return deps.ChatReads != nil && deps.ChatLogs != nil && deps.Lineage != nil
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

// listWorkspaces folds each visible workspace's chats in rather than giving chats their own tool, to
// stay under the tool ceiling. It takes no argument a model could widen c.Visible with; chats arrive in
// one read, bucketed in Go.
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
	return render.RenderWorkspaces(c.Workspace, visible, chatsByWorkspace(all, visible)), nil
}

// chatsByWorkspace buckets a whole-table chat read by workspace, keeping only visible ones: the filter
// is an authority boundary. Every visible workspace gets an entry, so one with no chats still renders.
func chatsByWorkspace(
	all []domain.Chat,
	visible []domain.Workspace,
) map[string][]domain.Chat {
	out := make(map[string][]domain.Chat, len(visible))
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
		name: "get_chat_log",
		description: "Read the conversation of another chat you can see — a chat this one is threaded under, " +
			"or any other chat in reach. This chat's OWN threads are not readable. Get chat ids from list_workspaces.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"chatId":{"type":"string","description":"The chat to read. Get chat ids from list_workspaces."},
				"offset":{"type":"integer","minimum":0,"description":"How many of the newest turns to skip, paging BACKWARDS into older history. Defaults to 0, the latest turn; the reply says what to pass next."},
				"limit":{"type":"integer","minimum":1,"description":"How many turns to return. Defaults to 20, capped at 50."}
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
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

// getChatLog reads another chat only after two checks, both before ChatLogs touches disk: the chat's
// workspace must pass CanSee (deliberately wide: siblings are the point), then it must not be the
// caller's own descendant. offset and limit page the rendering only, never the scope.
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
	if err := refuseOwnThread(ctx, deps, c, in.ChatID); err != nil {
		return "", err
	}
	turns, err := deps.ChatLogs.ReadChatLog(ctx, in.ChatID)
	if err != nil {
		return "", fmt.Errorf("agenttools: get_chat_log: %w", err)
	}
	// The empty case is worded here only, whatever the ChatLogReader.
	if len(turns) == 0 {
		return NoChatTurnsText, nil
	}
	w := recentWindow(len(turns), in.Offset, in.Limit, defaultChatLogTurns, maxChatLogTurns)
	note := recentNote("turns", "get_chat_log", w)
	if w.empty() {
		return note, nil
	}
	return note + render.RenderChatLog(turns[w.start:w.end]), nil
}

// refuseOwnThread rejects a chat below the caller's own, however many folders deep, by finding the
// caller in the target's lineage. An unreadable lineage refuses, as CanSee does: unreadable is not empty.
func refuseOwnThread(
	ctx context.Context,
	deps Deps,
	c Caller,
	chatID string,
) error {
	ancestors, err := deps.Lineage.Ancestors(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agenttools: get_chat_log: lineage: %w", errors.Join(ErrOwnThread, err))
	}
	if slices.Contains(ancestors, c.ChatID) {
		return fmt.Errorf("agenttools: get_chat_log: %w", ErrOwnThread)
	}
	return nil
}
