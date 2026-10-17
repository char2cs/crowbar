package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// pathsReport is read by the desktop shell so it never computes the home or
// the socket itself.
type pathsReport struct {
	Home   string `json:"home"`
	Socket string `json:"socket"`
}

func newPathsCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "paths",
		Short:        "Print the resolved Crowbar home and daemon socket as JSON",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := home.Resolve()
			if err != nil {
				return fmt.Errorf("paths: %w", err)
			}
			report := pathsReport{Home: h.Root(), Socket: h.Socket()}
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(report); err != nil {
				return fmt.Errorf("paths: write: %w", err)
			}
			return nil
		},
	}
}
