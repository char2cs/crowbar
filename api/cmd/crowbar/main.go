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
	"github.com/char2cs/crowbar/api/internal/core/loopback"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/core/shellenv"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "crowbar",
		Short: "Crowbar — self-improving agentic development platform",
	}
	root.AddCommand(newServeCmd(), newVersionCmd(), newHookCmd(), newHandoffCmd(), newChatCmd())
	return root
}

func newServeCmd() *cobra.Command {
	var host string
	var loopbackTCP bool
	var loopbackTCPAddr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Crowbar daemon",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServe(host, loopback.Address(loopbackTCP, loopbackTCPAddr))
		},
	}
	cmd.Flags().StringVar(&host, "host", "unix://", "listen address (unix:// or tcp://127.0.0.1:3737; non-loopback TCP exposes the unauthenticated API to the network)")
	cmd.Flags().BoolVar(&loopbackTCP, "loopback-tcp", false,
		"additionally serve the API on a token-authenticated loopback TCP port (env "+loopback.EnvEnable+
			"); the port and token are published to <crowbar home>/state/"+loopback.FileName)
	cmd.Flags().StringVar(&loopbackTCPAddr, "loopback-tcp-addr", "",
		"bind address for --loopback-tcp; must be a literal loopback IP (default "+loopback.DefaultAddress+
			", an OS-assigned port; env "+loopback.EnvAddress+"). Setting it implies --loopback-tcp")
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
	loopbackAddr string,
) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Apply the user's login-shell PATH process-wide BEFORE anything execs
	// external tools or spawns PTYs. A daemon launched by macOS launchd (the
	// packaged .app) inherits a minimal PATH without Homebrew/npm/go dirs —
	// without this, gh/glab, language servers and terminal children all see a
	// crippled environment. Degrades to the inherited PATH on failure.
	shellenv.ApplyLoginShellPath(ctx)

	staticFS, err := embeddedStaticFS()
	if err != nil {
		return err
	}

	container, err := internal.New(ctx, host, staticFS, internal.WithLoopbackTCP(loopbackAddr))
	if err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}
	defer container.Close()

	fmt.Printf("crowbar listening on %s\n", host)
	// The bound port is announced; the token that guards it is NOT, here or
	// anywhere else. It reaches its clients through the 0600 credentials file only,
	// because stdout is captured by the desktop supervisor and by launchd, which
	// would put the secret in a log file the socket's 0600 mode exists to avoid.
	if addr := container.LoopbackAddress(); addr != "" {
		fmt.Printf("crowbar loopback API on http://%s (token: %s)\n", addr, container.LoopbackCredentialsPath())
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
