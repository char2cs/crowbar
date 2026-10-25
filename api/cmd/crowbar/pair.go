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
	Code              string `json:"code"`
	ServerFingerprint string `json:"serverFingerprint"`
}

// runPair mints a pairing code and prints it, alongside the daemon's TLS
// fingerprint, for the operator to hand to a client. The fingerprint is the
// trust-on-first-connect check: a human compares it against what the
// pairing client shows for the host it just dialed, since the code alone
// only proves possession of this socket, not which daemon a later TLS
// connection actually reaches.
func runPair(socket string, out io.Writer) error {
	got, err := postJSON[mintCodeResponse](socket, "/v0/pair/code")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, `
Pairing code    %s
Fingerprint     %s

Enter both in the Crowbar client within %s. The code works once.

`, got.Code, got.ServerFingerprint, pairing.CodeTTL)
	return nil
}
