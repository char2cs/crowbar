package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

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
			if err := deps.Chats.RenameByRunner(ctx, c.RunnerID, title, "agent"); err != nil {
				return "", fmt.Errorf("agenttools: set_chat_title: %w", err)
			}
			return "Chat titled: " + title, nil
		},
	}}
}
