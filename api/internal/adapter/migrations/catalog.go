// Package migrations is Crowbar's catalog of migration steps: what each step
// is, when in the boot it runs and what it would do, and the implementations
// of the steps that run before any store is open.
package migrations

import (
	"context"

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
// binary must not migrate comes back as a live session whose Halt says why.
func Begin(
	ctx context.Context,
	h home.Home,
	opts migrate.Options,
) (*migrate.Session, error) {
	cat, err := Catalog()
	if err != nil {
		return nil, err
	}
	return migrate.Begin(ctx, h, cat, opts)
}

type planFunc = func(ctx context.Context, env migrate.Env) (migrate.Plan, error)

func legacySteps() []migrate.Step {
	return []migrate.Step{
		legacyStep(StepProjectHomes, migrate.PostOpen, rewrites("projects without a home workspace")),
		legacyStep(StepWorkspacePathsTable, migrate.PreOpen, planWorkspacePaths),
		legacyStep(StepChatTypes, migrate.PostOpen, rewrites("chats minted before chat types existed")),
		legacyStep(StepChatProviders, migrate.PostOpen, rewrites("chat providers their runner history disagrees with")),
		legacyStep(StepHookDeliveries, migrate.PostOpen, deletes(".hook-deliveries directories")),
	}
}

// legacyStep plans the adoption of the step's marker where an older daemon
// left one, or an earlier attempt recorded adopting it, and plan's work
// everywhere else.
func legacyStep(
	number int,
	phase migrate.Phase,
	plan planFunc,
) migrate.Step {
	name := legacyNames[number]
	return migrate.Step{
		Number: number,
		Name:   name,
		Phase:  phase,
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
