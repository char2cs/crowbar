package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
)

func newPairCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pair",
		Short: "Mint a one-time pairing code for connecting a Crowbar client (needs device connections on)",
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
	Code              string   `json:"code"`
	ServerFingerprint string   `json:"serverFingerprint"`
	Addresses         []string `json:"addresses"`
}

func runPair(socket string, out io.Writer) error {
	got, err := postJSON[mintCodeResponse](socket, "/v0/pair/code")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, `
Pairing code    %s
Address         %s
Fingerprint     %s

Enter both in the Crowbar client within %s. The code works once.

`, got.Code, strings.Join(got.Addresses, ", "), got.ServerFingerprint, pairing.CodeTTL)
	if err != nil {
		return fmt.Errorf("pair: print the code: %w", err)
	}
	return nil
}
