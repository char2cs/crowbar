package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
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

	got, err := run(context.Background(), seedOptions{
		WorkspaceID:      *workspaceID,
		ChatID:           *chatID,
		Turns:            *turns,
		ToolCallsPerTurn: *toolCalls,
	})
	if errors.Is(err, migrate.ErrHomeLocked) {
		log.Fatalf("crowbar-seed-chat: open storage (is the dev daemon still running against this CROWBAR_HOME? stop it first): %v", err)
	}
	if err != nil {
		log.Fatalf("crowbar-seed-chat: %v", err)
	}
	fmt.Printf("seeded chat %s (%d turns, %d tool calls/assistant turn)\n", got, *turns, *toolCalls)
}

// run seeds the chat into the home a daemon has already migrated, holding it as
// the daemon does: for as long as its stores are open.
func run(
	ctx context.Context,
	opts seedOptions,
) (chatID string, err error) {
	h, err := home.Resolve()
	if err != nil {
		return "", err
	}
	if err := requireMigrated(ctx, h); err != nil {
		return "", err
	}
	session, err := migrations.Begin(ctx, h, migrate.Options{Version: metadata.GetVersion()})
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, session.Close()) }()
	if halt := session.Halt(); halt != nil {
		return "", halt
	}
	adapters, err := adapter.New(adapter.WithHomeDir(h.Root()))
	if err != nil {
		return "", fmt.Errorf("open storage: %w", err)
	}
	defer func() { err = errors.Join(err, adapters.Close()) }()
	return seedChat(ctx, adapters, opts)
}

// requireMigrated refuses a home with migration steps left: running them is the
// daemon's boot, and a seeder that started a run and left it unfinished would
// have the next boot close that run as interrupted.
func requireMigrated(
	ctx context.Context,
	h home.Home,
) error {
	cat, err := migrations.Catalog()
	if err != nil {
		return err
	}
	report, err := migrate.Inspect(ctx, h, cat, migrate.Options{})
	if err != nil {
		return fmt.Errorf("inspect %s: %w", h.Root(), err)
	}
	if len(report.Steps) == 0 {
		return nil
	}
	names := make([]string, 0, len(report.Steps))
	for _, sp := range report.Steps {
		names = append(names, sp.Step.Name)
	}
	return fmt.Errorf("%s has migration steps pending (%s): start the daemon once to finish migrating this home",
		h.Root(), strings.Join(names, ", "))
}
