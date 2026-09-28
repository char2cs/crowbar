package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
)

func newPairCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pair",
		Short: "Mint a one-time pairing code for connecting a Crowbar client",
		RunE: func(_ *cobra.Command, _ []string) error {
			socket, err := transports.SocketPath("unix://")
			if err != nil {
				return err
			}
			return runPair(socket, os.Stdout)
		},
	}
}

type mintCodeResponse struct {
	Code string `json:"code"`
}

// runPair mints a pairing code and prints it for the operator to hand to a
// client, alongside its expiry.
func runPair(socket string, out io.Writer) error {
	got, err := postJSON[mintCodeResponse](socket, "/v0/pair/code")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, `
Pairing code    %s

Enter it in the Crowbar client within %s. It works once.

`, got.Code, pairing.CodeTTL)
	return nil
}
