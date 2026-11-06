// Package migrations is Crowbar's catalog of migration steps: what each step
// is, when in the boot it runs and what it would do, and the implementations
// of the steps that run before any store is open.
package migrations

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations/chatledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

// The legacy steps: the one-off boot upgrades the pre-engine daemon ran and
// marked done under state/upgrades, numbered in the order it ran them.
const (
	StepProjectHomes        = 1
	StepWorkspacePathsTable = 2
	StepChatTypes           = 3
	StepChatProviders       = 4
	StepHookDeliveries      = 5
)

// StepChatLedger restates the legacy chat records as ledger rows. It changes
// no layout, so the post-open legacy steps after it stay as they are.
const StepChatLedger = 6

// StepPlacementTree copies every sidebar placement into the per-project Trees.
// It deletes nothing: the node files and the folders table stay for a rollback.
const StepPlacementTree = 7

// StepLedgerLayout bumps the layout to 1, the home of the chat ledger and the
// project trees: a build that knows only layout 0 halts on it instead of
// re-creating the legacy files StepRetireLegacy deletes.
const StepLedgerLayout = 9

// StepRetireLegacy deletes the legacy files once the migrated home has booted,
// keeping any its backup does not hold as it is.
const StepRetireLegacy = 10

// legacyNames are the steps' names, which were also their markers' file names.
var legacyNames = map[int]string{
	StepProjectHomes:        "project-homes",
	StepWorkspacePathsTable: "workspace-paths-table",
	StepChatTypes:           "chat-types",
	StepChatProviders:       "chat-providers-from-history",
	StepHookDeliveries:      "hook-deliveries",
}

// Catalog is every migration step this binary knows.
func Catalog() (*migrate.Catalog, error) {
	return migrate.NewCatalog(legacySteps()...)
}

// Begin takes h for one boot against Catalog; see migrate.Begin. A home this
// binary must not migrate comes back as a live session whose Halt says why,
// and a step failing on a store fault halts in the words storesqlite has for it.
func Begin(
	ctx context.Context,
	h home.Home,
	opts migrate.Options,
) (*migrate.Session, error) {
	cat, err := Catalog()
	if err != nil {
		return nil, err
	}
	opts.Explain = storesqlite.Explain
	return migrate.Begin(ctx, h, cat, opts)
}

type planFunc = func(ctx context.Context, env migrate.Env) (migrate.Plan, error)

func legacySteps() []migrate.Step {
	return []migrate.Step{
		legacyStep(StepProjectHomes, migrate.PostOpen, 1, rewrites("projects without a home workspace")),
		legacyStep(StepWorkspacePathsTable, migrate.PreOpen, 0, planWorkspacePaths),
		legacyStep(StepChatTypes, migrate.PostOpen, 1, rewrites("chats minted before chat types existed")),
		legacyStep(StepChatProviders, migrate.PostOpen, 1, rewrites("chat providers their runner history disagrees with")),
		legacyStep(StepHookDeliveries, migrate.PostOpen, 1, deletes(".hook-deliveries directories")),
		// At layout 1, so it recurs after the bump: a home an older build wrote
		// legacy records into again is converted at its next boot.
		{Number: StepChatLedger, Name: "chat-ledger", Phase: migrate.PreOpen, From: 1, To: 1, Plan: chatledger.Plan, Recurs: true},
		{Number: StepPlacementTree, Name: "placement-tree", Phase: migrate.PostOpen, From: 1, To: 1, Plan: planPlacementTree},
		{Number: StepChatAttachments, Name: "chat-attachments", Phase: migrate.PreOpen, Plan: planChatAttachments},
		{Number: StepLedgerLayout, Name: "ledger-layout", Phase: migrate.PreOpen, From: 0, To: 1, Plan: planLedgerLayout},
		{Number: StepRetireLegacy, Name: "retire-legacy", Phase: migrate.PreOpen, From: 1, To: 1, Plan: planRetire, Recurs: true},
		{Number: StepOwnerOnly, Name: "owner-only", Phase: migrate.PreOpen, From: 1, To: 1, Plan: planOwnerOnly},
	}
}

// legacyStep plans the adoption of the step's marker where an older daemon
// left one, or an earlier attempt recorded adopting it, and plan's work
// everywhere else.
func legacyStep(
	number int,
	phase migrate.Phase,
	layout int,
	plan planFunc,
) migrate.Step {
	name := legacyNames[number]
	return migrate.Step{
		Number: number,
		Name:   name,
		Phase:  phase,
		From:   layout,
		To:     layout,
		Plan: func(ctx context.Context, env migrate.Env) (migrate.Plan, error) {
			adopts, err := adopting(env.Home, name, env.Earlier.Done)
			switch {
			case err != nil:
				return migrate.Plan{}, err
			case adopts:
				return migrate.Plan{Actions: []migrate.Action{
					{Kind: migrate.Delete, Subject: markerSubject(name), Count: 1},
				}}, nil
			default:
				return plan(ctx, env)
			}
		},
	}
}

func rewrites(
	subject string,
) planFunc {
	return func(context.Context, migrate.Env) (migrate.Plan, error) {
		return migrate.Plan{
			Actions:       []migrate.Action{{Kind: migrate.Convert, Subject: subject, Count: migrate.Unknown}},
			RewritesState: true,
		}, nil
	}
}

// planLedgerLayout is the bump alone: the engine writes LAYOUT when it is done.
func planLedgerLayout(context.Context, migrate.Env) (migrate.Plan, error) {
	return migrate.Plan{Actions: []migrate.Action{{Kind: migrate.Record, Subject: "LAYOUT 0 -> 1", Count: 1}}}, nil
}

// deletes is work outside state/, so it needs no backup of it.
func deletes(
	subject string,
) planFunc {
	return func(context.Context, migrate.Env) (migrate.Plan, error) {
		return migrate.Plan{
			Actions: []migrate.Action{{Kind: migrate.Delete, Subject: subject, Count: migrate.Unknown}},
		}, nil
	}
}
