package agents

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// StartServe starts cmd, a descriptor's runtime.api.serve, with its stdin and
// stdout as the stdio StartAPIConn speaks over; its stderr stays cmd's own.
// The pipes exist before serve does, so nothing written to them is early.
func StartServe(cmd *exec.Cmd) (APIStdio, error) {
	childIn, in, err := os.Pipe()
	if err != nil {
		return APIStdio{}, fmt.Errorf("agents: serve stdin: %w", err)
	}
	out, childOut, err := os.Pipe()
	if err != nil {
		return APIStdio{}, errors.Join(fmt.Errorf("agents: serve stdout: %w", err), childIn.Close(), in.Close())
	}
	cmd.Stdin, cmd.Stdout = childIn, childOut
	if err := cmd.Start(); err != nil {
		return APIStdio{}, errors.Join(fmt.Errorf("agents: start serve: %w", err),
			childIn.Close(), childOut.Close(), in.Close(), out.Close())
	}
	// Only serve may hold its own ends: one left open here would hide its exit,
	// which reaches the reader as EOF.
	if err := errors.Join(childIn.Close(), childOut.Close()); err != nil {
		// Nothing can speak to this serve, so it is ended and reaped here.
		return APIStdio{}, errors.Join(fmt.Errorf("agents: serve pipes: %w", err),
			in.Close(), out.Close(), cmd.Process.Kill(), cmd.Wait())
	}
	return APIStdio{In: in, Out: out}, nil
}
