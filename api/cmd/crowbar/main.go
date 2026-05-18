package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
	Run: func(
		cmd *cobra.Command,
		args []string,
	) {
		fmt.Println("crowbar dev")
	},
}

func init() {
	serveCmd.Flags().StringVar(&host, "host", "unix://", "listen address (unix:// or tcp://0.0.0.0:3737)")
	rootCmd.AddCommand(serveCmd, versionCmd)
}

func runServe(
	cmd *cobra.Command,
	args []string,
) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("crowbar starting on %s\n", host)
	<-ctx.Done()
	fmt.Println("crowbar stopping")
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
