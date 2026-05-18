package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/crowbar/api/internal"
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

	container, err := internal.New(ctx, host, nil)
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
