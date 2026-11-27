package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/core/shellenv"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "crowbar",
		Short: "Crowbar — self-improving agentic development platform",
	}
	root.AddCommand(newServeCmd(), newVersionCmd(), newHandoffCmd(), newMCPCmd(), newDescriptorCmd(), newPathsCmd(), newMigrateCmd(), newPairCmd())
	return root
}

func newServeCmd() *cobra.Command {
	var host string
	cmd := &cobra.Command{
		Use:          "serve",
		Short:        "Start the Crowbar daemon",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServe(host)
		},
	}
	cmd.Flags().StringVar(&host, "host", "unix://", "listen address: unix://; tcp://127.0.0.1:3737 is local development only — "+
		"plaintext and unauthenticated, never reverse-proxy or port-forward it; a non-loopback tcp:// address opens the device "+
		"listener at start (TLS, paired clients only) and also binds the local socket the daemon's own callbacks and crowbar pair dial; "+
		"device connections can also be switched at runtime from Settings")
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

	shellenv.ApplyLoginShellPath(ctx)

	staticFS, err := embeddedStaticFS()
	if err != nil {
		return err
	}
	return serve(ctx, host, staticFS, internal.WithLogRing(installLogRing()))
}

func serve(
	ctx context.Context,
	host string,
	staticFS fs.FS,
	options ...internal.Option,
) error {
	container, err := internal.New(ctx, host, staticFS, options...)
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		slog.Warn("daemon: boot interrupted", "component", "daemon", "err", err)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}
	defer container.Close() //nolint:contextcheck // teardown runs after ctx is cancelled; it must not inherit that cancellation

	slog.Info("daemon: listening", "component", "daemon", "version", metadata.GetVersion(),
		"host", host, "address", container.Address(), "pid", os.Getpid(), "home", container.Home())
	if halt := container.Halt(); halt != nil {
		slog.Error("daemon: halted", "component", "daemon", "reason", string(halt.Reason),
			"message", halt.Message, "ledger", halt.Ledger, "backup", halt.Backup)
	}
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
