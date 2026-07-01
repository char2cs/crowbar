package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "crowbar",
		Short: "Crowbar — self-improving agentic development platform",
	}
	root.AddCommand(newServeCmd(), newVersionCmd())
	return root
}

func newServeCmd() *cobra.Command {
	var host string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Crowbar daemon",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServe(host)
		},
	}
	cmd.Flags().StringVar(&host, "host", "unix://", "listen address (unix:// or tcp://127.0.0.1:3737; non-loopback TCP exposes the unauthenticated API to the network)")
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println("crowbar " + metadata.GetVersion())
		},
	}
}

func runServe(
	host string,
) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	staticFS, err := embeddedStaticFS()
	if err != nil {
		return err
	}

	container, err := internal.New(ctx, host, staticFS)
	if err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}
	defer container.Close()

	fmt.Printf("crowbar listening on %s\n", host)
	return container.Run(ctx)
}

func embeddedStaticFS() (fs.FS, error) {
	if _, err := embeddedWeb.Open("."); err != nil {
		return nil, nil
	}
	sub, err := fs.Sub(embeddedWeb, "web/dist")
	if err != nil {
		return nil, fmt.Errorf("embedded web assets: %w", err)
	}
	return sub, nil
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
