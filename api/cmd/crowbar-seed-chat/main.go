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
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

func main() {
	workspaceID := flag.String("workspace-id", "", "existing workspace ID to attach the seeded chat to (required)")
	chatID := flag.String("chat-id", "", "chat ID to use (default: generated)")
	turns := flag.Int("turns", 20, "number of user/assistant turn pairs")
	toolCalls := flag.Int("tool-calls", 5, "finished tool calls per assistant turn")
	provider := flag.String("provider", "", "provider the chat runs on (default: the first registered descriptor)")
	flag.Parse()

	if *workspaceID == "" {
		log.Fatal("crowbar-seed-chat: --workspace-id is required")
	}

	got, err := run(context.Background(), seedOptions{
		WorkspaceID:      *workspaceID,
		ChatID:           *chatID,
		ProviderID:       *provider,
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

// run seeds the chat into the home a daemon has already migrated.
func run(
	ctx context.Context,
	opts seedOptions,
) (string, error) {
	h, err := home.Resolve()
	if err != nil {
		return "", err
	}
	if err := requireMigrated(ctx, h); err != nil {
		return "", err
	}
	return seedHeld(ctx, h, opts)
}

// seedHeld seeds holding the home, as the daemon does, while its stores are
// open. The home can change after requireMigrated looked, so it looks again
// under the lock: no store opens on a halted home or one with steps pending.
func seedHeld(
	ctx context.Context,
	h home.Home,
	opts seedOptions,
) (chatID string, err error) {
	session, err := migrations.Begin(ctx, h, migrate.Options{Version: metadata.GetVersion()})
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, session.Close()) }()
	if halt := session.Halt(); halt != nil {
		return "", halt
	}
	if err := requireMigrated(ctx, h); err != nil {
		return "", err
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
	report, err := migrate.Inspect(ctx, h, cat, app.Implemented, migrate.Options{})
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
