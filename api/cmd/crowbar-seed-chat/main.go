package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/char2cs/crowbar/api/internal/adapter"
)

func main() {
	workspaceID := flag.String("workspace-id", "", "existing workspace ID to attach the seeded chat to (required)")
	chatID := flag.String("chat-id", "", "chat ID to use (default: generated)")
	turns := flag.Int("turns", 20, "number of user/assistant turn pairs")
	toolCalls := flag.Int("tool-calls", 5, "finished tool calls per assistant turn")
	flag.Parse()

	if *workspaceID == "" {
		log.Fatal("crowbar-seed-chat: --workspace-id is required")
	}

	adapters, err := adapter.New()
	if err != nil {
		log.Fatalf("crowbar-seed-chat: open storage (is the dev daemon still running against this CROWBAR_HOME? stop it first): %v", err)
	}
	defer func() { _ = adapters.Close() }()

	got, err := seedChat(context.Background(), adapters, seedOptions{
		WorkspaceID:      *workspaceID,
		ChatID:           *chatID,
		Turns:            *turns,
		ToolCallsPerTurn: *toolCalls,
	})
	if err != nil {
		log.Fatalf("crowbar-seed-chat: %v", err)
	}
	fmt.Printf("seeded chat %s (%d turns, %d tool calls/assistant turn)\n", got, *turns, *toolCalls)
}
