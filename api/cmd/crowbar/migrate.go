package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

// newMigrateCmd has no apply path on purpose: the daemon migrates the home as
// it boots, under the home lock, and this command only shows what that would do.
func newMigrateCmd() *cobra.Command {
	var plan bool
	cmd := &cobra.Command{
		Use:          "migrate --plan",
		Short:        "Preview the migration steps the daemon would run on the next boot",
		Long:         "Print the layout, the pending migration steps with their actions and counts, and whether state/ would be backed up. Nothing on disk is changed.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !plan {
				return errors.New("applying happens when the daemon boots; use --plan to preview")
			}
			return runMigratePlan(cmd)
		},
	}
	cmd.Flags().BoolVar(&plan, "plan", false, "print the pending migration steps without changing anything")
	return cmd
}

func runMigratePlan(
	cmd *cobra.Command,
) error {
	h, err := home.Resolve()
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	cat, err := migrations.Catalog()
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	report, err := migrate.Inspect(cmd.Context(), h, cat, app.Implemented, migrate.Options{})
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := report.Render(cmd.OutOrStdout()); err != nil {
		return fmt.Errorf("migrate: write: %w", err)
	}
	return nil
}
