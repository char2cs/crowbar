package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"github.com/char2cs/crowbar/api/internal"
	"github.com/spf13/cobra"
)

var host string

var rootCmd = &cobra.Command{
	Use:   "crowbar",
	Short: "Crowbar — self-improving agentic development platform",
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Crowbar daemon",
	RunE:  runServe,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("crowbar dev")
	},
}

func init() {
	serveCmd.Flags().StringVar(&host, "host", "unix://", "listen address (unix:// or tcp://0.0.0.0:3737)")
	rootCmd.AddCommand(serveCmd, versionCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// embeddedWeb is a zero-value embed.FS when built with -tags noEmbed (dev mode).
	// In that case we skip static serving; the Vite dev server handles the frontend.
	var staticFS fs.FS
	if _, err := embeddedWeb.Open("."); err == nil {
		sub, err := fs.Sub(embeddedWeb, "web/dist")
		if err != nil {
			return fmt.Errorf("embedded web assets: %w", err)
		}
		staticFS = sub
	}

	container, err := internal.New(ctx, host, staticFS)
	if err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}
	defer container.Close()

	fmt.Printf("crowbar listening on %s\n", host)
	return container.Run(ctx)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
