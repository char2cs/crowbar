package migrations

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations/chatledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

const (
	StepProjectHomes        = 1
	StepWorkspacePathsTable = 2
	StepChatTypes           = 3
	StepChatProviders       = 4
	StepHookDeliveries      = 5
)

const StepChatLedger = 6

const StepPlacementTree = 7

const StepLedgerLayout = 9

const StepRetireLegacy = 10

var legacyNames = map[int]string{
	StepProjectHomes:        "project-homes",
	StepWorkspacePathsTable: "workspace-paths-table",
	StepChatTypes:           "chat-types",
	StepChatProviders:       "chat-providers-from-history",
	StepHookDeliveries:      "hook-deliveries",
}

func Catalog() (*migrate.Catalog, error) {
	return migrate.NewCatalog(legacySteps()...)
}

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
		{Number: StepChatLedger, Name: "chat-ledger", Phase: migrate.PreOpen, From: 1, To: 1, Plan: chatledger.Plan, Recurs: true},
		{Number: StepPlacementTree, Name: "placement-tree", Phase: migrate.PostOpen, From: 1, To: 1, Plan: planPlacementTree},
		{Number: StepChatAttachments, Name: "chat-attachments", Phase: migrate.PreOpen, Plan: planChatAttachments},
		{Number: StepLedgerLayout, Name: "ledger-layout", Phase: migrate.PreOpen, From: 0, To: 1, Plan: planLedgerLayout},
		{Number: StepRetireLegacy, Name: "retire-legacy", Phase: migrate.PreOpen, From: 1, To: 1, Plan: planRetire, Recurs: true},
		{Number: StepOwnerOnly, Name: "owner-only", Phase: migrate.PreOpen, From: 1, To: 1, Plan: planOwnerOnly},
		{Number: StepWorktreeReach, Name: "worktree-reach", Phase: migrate.PreOpen, From: 1, To: 1, Plan: planWorktreeReach},
	}
}

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

func planLedgerLayout(context.Context, migrate.Env) (migrate.Plan, error) {
	return migrate.Plan{Actions: []migrate.Action{{Kind: migrate.Record, Subject: "LAYOUT 0 -> 1", Count: 1}}}, nil
}

func deletes(
	subject string,
) planFunc {
	return func(context.Context, migrate.Env) (migrate.Plan, error) {
		return migrate.Plan{
			Actions: []migrate.Action{{Kind: migrate.Delete, Subject: subject, Count: migrate.Unknown}},
		}, nil
	}
}
