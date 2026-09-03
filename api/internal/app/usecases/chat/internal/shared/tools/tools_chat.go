package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ChatRenamer is the narrow write port set_chat_title needs from the agent
// usecase: renaming by runner id, not chat id, so the tool always affects
// whichever chat the calling runner is on right now.
type ChatRenamer interface {
	RenameByRunner(ctx context.Context, runnerID, title, source string) error
}

func chatTools(deps Deps) []toolDef {
	if deps.Chats == nil {
		return nil
	}
	return []toolDef{{
		name:        "set_chat_title",
		description: "Set this chat's title. Call once, early, with a concise 2-5 word Title-Case summary of the task.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{"title":{"type":"string","description":"Concise 2-5 word Title-Case title."}},
			"required":["title"],
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			var in struct {
				Title string `json:"title"`
			}
			if err := decode(args, &in); err != nil {
				return "", err
			}
			title := strings.TrimSpace(in.Title)
			if title == "" {
				return "", fmt.Errorf("agenttools: set_chat_title: title must not be empty")
			}
			// Renaming by RUNNER, not by chat id: the runner resolves to whatever
			// chat it is on right now, so an agent that cleared its conversation
			// titles the chat it is actually in.
			//
			// source="agent" gives agent precedence: it upgrades a derived title
			// and never clobbers one the user locked.
			if err := deps.Chats.RenameByRunner(ctx, c.RunnerID, title, "agent"); err != nil {
				return "", fmt.Errorf("agenttools: set_chat_title: %w", err)
			}
			return "Chat titled: " + title, nil
		},
	}}
}
